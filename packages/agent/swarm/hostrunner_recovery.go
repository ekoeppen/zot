package swarm

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/patriceckhart/zot/packages/continuous"
	"github.com/patriceckhart/zot/packages/core"
)

var errHostDisconnected = errors.New("swarm: host disconnected")

func hostTransportError(err error) bool {
	var network net.Error
	return errors.Is(err, errHostDisconnected) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) || errors.As(err, &network)
}

type hostRunState struct {
	incarnation  string
	attached     bool
	initialAdded bool
	seen         map[string]bool
	prompts      map[string]Prompt
	submissions  map[string]string
}

// Run reconnects after transport loss without restarting or duplicating host
// work. Stable host request IDs also cover a lost submission acknowledgement.
func (r *HostRunner) Run(ctx context.Context, sink Sink) error {
	state := &hostRunState{incarnation: rand.Text(), seen: map[string]bool{}, prompts: map[string]Prompt{}, submissions: map[string]string{}}
	recovering := false
	delay := 100 * time.Millisecond
	for {
		err := r.runConnected(ctx, sink, state)
		if err == nil || ctx.Err() != nil {
			return err
		}
		if !errors.Is(err, errHostDisconnected) && !(recovering && hostTransportError(err)) {
			return err
		}
		recovering = true
		r.agent.mu.Lock()
		r.agent.ready = false
		r.agent.mu.Unlock()
		sink.Activity("reconnecting to host")
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if delay < 5*time.Second {
			delay = min(delay*2, 5*time.Second)
		}
	}
}

func (s *hostRunState) mirror(r *HostRunner, entries []continuous.Entry, sink Sink, emit func(string, map[string]any)) {
	for _, entry := range entries {
		if s.seen[entry.ID] {
			continue
		}
		// Entry IDs are durable, so resnapshot and reconnect do not repeat output.
		emitEntry(func(typ string, data map[string]any) {
			data["entry_id"] = entry.ID
			emit(typ, data)
		}, entry)
		r.mirrorEntries([]continuous.Entry{entry}, sink, s.seen)
	}
}

// Reconcile submissions, not just the latest run. A watch gap or a sleeping
// laptop can miss the settlement commit even though work finished on the host.
func (s *hostRunState) reconcile(ctx context.Context, client *continuous.Client, child string, r *HostRunner, sink Sink, emit func(string, map[string]any)) error {
	var settled []continuous.Submission
	for id := range s.submissions {
		var sub continuous.Submission
		if err := client.CallInto(ctx, "submission.get", map[string]any{"id": id}, &sub); err != nil {
			return fmt.Errorf("swarm: reconcile submission: %w", err)
		}
		if sub.State != "queued" && sub.State != "running" {
			settled = append(settled, sub)
		}
	}
	if len(settled) == 0 {
		return nil
	}
	slices.SortFunc(settled, func(a, b continuous.Submission) int {
		if a.Sequence < b.Sequence {
			return -1
		}
		if a.Sequence > b.Sequence {
			return 1
		}
		return 0
	})
	var snap continuous.ConversationSnapshot
	if err := client.CallInto(ctx, "conversation.snapshot", map[string]any{"id": child, "limit": 1000}, &snap); err != nil {
		return err
	}
	// Snapshots contain active tasks only. Terminal generations are needed
	// when another turn finished while this supervisor was disconnected.
	var history struct {
		Tasks []continuous.Task `json:"tasks"`
	}
	if err := client.CallInto(ctx, "task.list", map[string]any{"conversation": child}, &history); err != nil {
		return fmt.Errorf("swarm: recover generation history: %w", err)
	}
	snap.Tasks = history.Tasks
	s.mirror(r, snap.Entries, sink, emit)
	for _, sub := range settled {
		requestID := s.submissions[sub.ID]
		response, errMsg := hostSubmissionResult(snap, sub)
		wireID := requestID
		if wireID == "task" {
			wireID = ""
		}
		data := map[string]any{"step": float64(1), "request_id": wireID, "response": response, "error": errMsg, "submission_id": sub.ID}
		emit("turn_end", data)
		notifyPromptTurnEnd(r.agent, NewEvent("turn_end", data))
		delete(s.prompts, requestID)
		delete(s.submissions, sub.ID)
	}
	return nil
}

// A successful generation commits its final assistant entry and terminal task
// atomically. Terminal tasks discard checkpoints, so correlate by that commit's
// revision, never by a checkpoint or the latest response of a different run.
func hostSubmissionResult(snap continuous.ConversationSnapshot, sub continuous.Submission) (string, string) {
	var revision uint64
	reason := ""
	for _, task := range snap.Tasks {
		if task.Kind != continuous.TaskKindGeneration || task.State != "terminal" {
			continue
		}
		var result struct {
			Submissions []string `json:"submissions"`
		}
		if json.Unmarshal(task.Result, &result) != nil || !slices.Contains(result.Submissions, sub.ID) {
			continue
		}
		revision, reason = task.Revision, task.Error
		break
	}
	if revision == 0 && snap.Run != nil && snap.Run.Phase == "done" && slices.Contains(snap.Run.Submissions, sub.ID) {
		revision, reason = snap.Run.Revision, snap.Run.Error
	}
	errMsg := ""
	if sub.State != "answered" {
		errMsg = strings.TrimSuffix(sub.State+": "+reason, ": ")
	} else if revision == 0 {
		return "", "host completion metadata unavailable, inspect the host transcript before explicitly retrying"
	}
	for _, entry := range snap.Entries {
		if entry.Type != "assistant" || entry.Revision != revision {
			continue
		}
		if msg, err := core.DecodeMessage(entry.Message); err == nil {
			return core.MessageText(msg), errMsg
		}
	}
	if sub.State == "answered" {
		errMsg = "host response unavailable in the retained snapshot window, inspect the host transcript before explicitly retrying"
	}
	return "", errMsg
}
