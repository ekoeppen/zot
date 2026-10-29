package swarm

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// QueuedTurn is the durable outcome of an automated follow-up. Dispatched work
// without a recorded completion is interrupted, never silently replayed.
type QueuedTurn struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	State    string `json:"state"`
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
	Reported bool   `json:"reported,omitempty"`
}

type queuedTurn struct {
	QueuedTurn
	complete func(QueuedTurn) bool
}

// QueueUserTurn retains the convenience API for callers needing only an error.
func (f *Swarm) QueueUserTurn(id, text string, complete func(error)) error {
	var callback func(QueuedTurn) bool
	if complete != nil {
		callback = func(turn QueuedTurn) bool {
			var err error
			if turn.Error != "" {
				err = errors.New(turn.Error)
			}
			complete(err)
			return true
		}
	}
	_, err := f.QueueTurn(id, text, callback)
	return err
}

// QueueTurn durably accepts a follow-up, including for stopped or detached
// agents. Resume starts dispatching pending work without replaying the task.
func (f *Swarm) QueueTurn(id, text string, complete func(QueuedTurn) bool) (string, error) {
	// Resume replaces the Agent and Remove deletes its files. Acceptance must
	// target the current instance, not persist into a retired worker's queue.
	f.resumeMu.Lock()
	defer f.resumeMu.Unlock()
	a := f.Get(id)
	if a == nil {
		return "", fmt.Errorf("no such agent %q", id)
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("message is required")
	}
	if len(text) > MaxPromptWireBytes {
		return "", errors.New("message is too large")
	}
	requestID := rand.Text()
	frame, err := json.Marshal(Prompt{Text: text, RequestID: requestID})
	if err != nil || len(frame) > MaxPromptWireBytes {
		return "", errors.New("encoded message is too large")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.queueLoadErr != nil {
		return "", a.queueLoadErr
	}
	turn := queuedTurn{QueuedTurn: QueuedTurn{ID: requestID, Text: text, State: "pending"}, complete: complete}
	a.followups = append(a.followups, turn)
	if err := a.writeQueueLocked(); err != nil {
		a.followups = a.followups[:len(a.followups)-1]
		return "", err
	}
	f.startQueueLocked(a)
	return turn.ID, nil
}

// QueuedTurns returns copies, without callback or transport state.
func (a *Agent) QueuedTurns() []QueuedTurn {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]QueuedTurn, 0, len(a.followups))
	for _, turn := range a.followups {
		out = append(out, turn.QueuedTurn)
	}
	return out
}

func (a *Agent) queuePath() string { return filepath.Join(filepath.Dir(a.EventLogPath), "queue.json") }

// Called with a.mu held. A failed durable write never permits a dispatch.
func (a *Agent) writeQueueLocked() error {
	if a.EventLogPath == "" {
		return errors.New("swarm: missing durable queue path")
	}
	turns := make([]QueuedTurn, 0, len(a.followups))
	for _, turn := range a.followups {
		turns = append(turns, turn.QueuedTurn)
	}
	path := a.queuePath()
	file, err := os.CreateTemp(filepath.Dir(path), ".queue-*")
	if err != nil {
		return fmt.Errorf("swarm queue create: %w", err)
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(struct {
		Version int          `json:"version"`
		Turns   []QueuedTurn `json:"turns"`
	}{1, turns})
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(file.Name(), path)
	}
	if err != nil {
		return fmt.Errorf("swarm queue save: %w", err)
	}
	return nil
}

func (a *Agent) loadQueue() error {
	data, err := os.ReadFile(a.queuePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("swarm queue read: %w", err)
	}
	var saved struct {
		Version int          `json:"version"`
		Turns   []QueuedTurn `json:"turns"`
	}
	if json.Unmarshal(data, &saved) != nil || saved.Version != 1 {
		return errors.New("swarm: invalid queue file")
	}
	seen := make(map[string]bool)
	for _, turn := range saved.Turns {
		if turn.ID == "" || seen[turn.ID] {
			return errors.New("swarm: invalid queue request id")
		}
		seen[turn.ID] = true
		switch turn.State {
		case "pending", "dispatched", "completed", "failed", "interrupted":
		default:
			return errors.New("swarm: invalid queue state")
		}
	}
	// The event log can contain a completion written just before a parent crash.
	// Recover that result, but never infer safe replay from a missing event.
	var events []Event
	for _, turn := range saved.Turns {
		if turn.State == "dispatched" {
			events, err = ReadEventLog(a.EventLogPath)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("swarm queue recovery: %w", err)
			}
			break
		}
	}
	for _, turn := range saved.Turns {
		if turn.State == "dispatched" {
			turn.State = "interrupted"
			turn.Error = "completion unknown after interruption, inspect the transcript before explicitly retrying"
			for _, ev := range events {
				id, _ := ev.Data["request_id"].(string)
				if id == turn.ID && ev.Type == "turn_end" {
					finishQueuedTurn(&turn, ev)
				}
			}
		}
		a.followups = append(a.followups, queuedTurn{QueuedTurn: turn})
	}
	return nil
}

func finishQueuedTurn(turn *QueuedTurn, ev Event) {
	turn.Error, _ = ev.Data["error"].(string)
	turn.Response, _ = ev.Data["response"].(string)
	turn.State = "completed"
	if turn.Error != "" {
		turn.State = "failed"
	}
}

func (f *Swarm) startQueueLocked(a *Agent) {
	if a.queueRunning || len(a.followups) == 0 || (a.status != StatusRunning && a.status != StatusPending) {
		return
	}
	a.queueRunning = true
	a.queueWG.Add(1)
	go f.runQueue(a)
}

func (a *Agent) signalTurnLocked() {
	a.queueWake++
	if a.turnChanged != nil {
		close(a.turnChanged)
	}
	a.turnChanged = make(chan struct{})
}

func (f *Swarm) runQueue(a *Agent) {
	defer a.queueWG.Done()
	skipped := map[string]bool{}
	var wake uint64
	for {
		a.mu.Lock()
		if wake != a.queueWake {
			clear(skipped)
			wake = a.queueWake
		}
		if a.status != StatusRunning && a.status != StatusPending {
			changed := false
			for idx := range a.followups {
				if a.followups[idx].State == "dispatched" {
					a.followups[idx].State = "interrupted"
					a.followups[idx].Error = "agent stopped before completion, inspect the transcript before retrying"
					changed = true
				}
			}
			if changed {
				if err := a.writeQueueLocked(); err != nil {
					a.lastErr = err
				}
			}
		}
		var report *queuedTurn
		for idx := range a.followups {
			turn := &a.followups[idx]
			if turn.State != "pending" && turn.State != "dispatched" && !turn.Reported && !skipped[turn.ID] && (turn.complete != nil || f.cfg.OnQueueResult != nil) {
				report = turn
				break
			}
		}
		if report != nil {
			result, callback := report.QueuedTurn, report.complete
			a.mu.Unlock()
			reported := false
			if callback != nil {
				reported = callback(result)
			} else {
				reported = f.cfg.OnQueueResult(a, result)
			}
			skipped[result.ID] = true
			a.mu.Lock()
			for idx := range a.followups {
				if a.followups[idx].ID == result.ID {
					a.followups[idx].Reported = reported
				}
			}
			err := a.writeQueueLocked()
			if err != nil {
				a.lastErr = err
				a.queueRunning = false
			}
			a.mu.Unlock()
			if err != nil {
				return
			}
			continue
		}
		if a.status != StatusRunning && a.status != StatusPending {
			a.queueRunning = false
			a.mu.Unlock()
			return
		}
		var next *queuedTurn
		for idx := range a.followups {
			if a.followups[idx].State == "pending" || a.followups[idx].State == "dispatched" {
				next = &a.followups[idx]
				break
			}
		}
		if next == nil {
			// Clear under the lock so a concurrent enqueue cannot miss starting a worker.
			a.queueRunning = false
			a.mu.Unlock()
			return
		}
		if a.ready && !a.busy && next.State == "pending" {
			next.State = "dispatched"
			err := a.writeQueueLocked()
			if err != nil {
				next.State = "pending"
				a.lastErr = err
				a.queueRunning = false
				a.mu.Unlock()
				return
			}
			id, text := next.ID, next.Text
			a.mu.Unlock()
			data, err := json.Marshal(Prompt{Text: text, RequestID: id})
			if err == nil {
				err = f.SendInput(a.ID, "user-prompt "+string(data))
			}
			if err != nil {
				a.mu.Lock()
				for idx := range a.followups {
					if a.followups[idx].ID == id && a.followups[idx].State == "dispatched" {
						a.followups[idx].State = "interrupted"
						a.followups[idx].Error = "delivery failed or uncertain: " + err.Error()
					}
				}
				if saveErr := a.writeQueueLocked(); saveErr != nil {
					a.lastErr = saveErr
				}
				a.mu.Unlock()
			}
			continue
		}
		if a.turnChanged == nil {
			a.turnChanged = make(chan struct{})
		}
		changed := a.turnChanged
		a.mu.Unlock()
		select {
		case <-changed:
			clear(skipped)
		case <-a.done:
		}
	}
}
