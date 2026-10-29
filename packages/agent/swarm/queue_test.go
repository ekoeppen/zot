package swarm

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type queueRunner struct{ input chan string }

func (r *queueRunner) Run(ctx context.Context, _ Sink) error { <-ctx.Done(); return ctx.Err() }
func (r *queueRunner) SendInput(s string) error              { r.input <- s; return nil }

func receiveQueued(t *testing.T, r *queueRunner, want string) Prompt {
	t.Helper()
	select {
	case msg := <-r.input:
		p, err := DecodePrompt(msg)
		if err != nil || p.Text != want || p.RequestID == "" {
			t.Fatalf("queued frame: %+v %v", p, err)
		}
		return p
	case <-time.After(3 * time.Second):
		t.Fatal("no queued message")
		return Prompt{}
	}
}
func finishQueue(a *Agent, p Prompt, response string) {
	notifyPromptTurnEnd(a, NewEvent("turn_end", map[string]any{"step": float64(1), "request_id": p.RequestID, "response": response}))
}
func stopQueued(t *testing.T, mgr *Swarm, a *Agent) {
	t.Helper()
	if err := mgr.Stop(a.ID); err != nil {
		t.Fatal(err)
	}
	a.Wait()
	a.queueWG.Wait()
}

func TestQueuedTurnsWaitAndSerialize(t *testing.T) {
	r := &queueRunner{input: make(chan string, 4)}
	mgr := New(Config{Root: t.TempDir(), RepoRoot: t.TempDir(), NewRunner: func(*Agent) Runner { return r }})
	a, err := mgr.Spawn(context.Background(), "initial")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopQueued(t, mgr, a) })
	completed := make(chan error, 2)
	for _, msg := range []string{"first", "second"} {
		if err := mgr.QueueUserTurn(a.ID, msg, func(err error) { completed <- err }); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case s := <-r.input:
		t.Fatalf("sent before readiness: %s", s)
	default:
	}
	notifyPromptTurnEnd(a, NewEvent("agent_ready", nil))
	first := receiveQueued(t, r, "first")
	// Initial, manual and provider-level completions cannot satisfy this request.
	notifyPromptTurnEnd(a, NewEvent("turn_end", map[string]any{"step": float64(1), "response": "manual answer"}))
	notifyPromptTurnEnd(a, NewEvent("turn_end", map[string]any{"stop": "tool_use"}))
	select {
	case s := <-r.input:
		t.Fatalf("unrelated completion advanced queue: %s", s)
	default:
	}
	finishQueue(a, first, "first answer")
	second := receiveQueued(t, r, "second")
	finishQueue(a, second, "second answer")
	for n := 0; n < 2; n++ {
		select {
		case err := <-completed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("no completion")
		}
	}
	turns := a.QueuedTurns()
	if turns[0].Response != "first answer" || turns[1].Response != "second answer" {
		t.Fatalf("wrong responses: %+v", turns)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(a.queuePath())
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("queue permissions: %v", info.Mode())
		}
	}
}

func TestLateCompletionSubscriber(t *testing.T) {
	a := &Agent{}
	notifyPromptTurnEnd(a, NewEvent("turn_end", map[string]any{"step": float64(1), "error": "failure"}))
	done := make(chan string, 1)
	a.SetOnTurnEnd(func(step int, msg string) {
		if step != 1 {
			msg = "wrong step"
		}
		done <- msg
	})
	select {
	case msg := <-done:
		if msg != "failure" {
			t.Fatal(msg)
		}
	case <-time.After(time.Second):
		t.Fatal("completion lost")
	}
}

func TestQueueSurvivesReloadAndResumeWithoutReplayingInterruptedWork(t *testing.T) {
	root, repo := t.TempDir(), t.TempDir()
	r := &queueRunner{input: make(chan string, 4)}
	cfg := Config{Root: root, RepoRoot: repo, NewRunner: func(*Agent) Runner { return r }}
	mgr := New(cfg)
	a, err := mgr.Spawn(context.Background(), "initial")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"possibly executed", "not sent yet"} {
		if _, err := mgr.QueueTurn(a.ID, text, nil); err != nil {
			t.Fatal(err)
		}
	}
	notifyPromptTurnEnd(a, NewEvent("agent_ready", nil))
	receiveQueued(t, r, "possibly executed")
	stopQueued(t, mgr, a)
	turns := a.QueuedTurns()
	if turns[0].State != "interrupted" || turns[1].State != "pending" {
		t.Fatalf("stop discarded work: %+v", turns)
	}
	results := make(chan QueuedTurn, 4)
	cfg.OnQueueResult = func(_ *Agent, turn QueuedTurn) bool { results <- turn; return true }
	restored := New(cfg)
	if n, errs := restored.Reload(); n != 1 || len(errs) != 0 {
		t.Fatalf("reload: %d %v", n, errs)
	}
	resumed, err := restored.Resume(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopQueued(t, restored, resumed) })
	notifyPromptTurnEnd(resumed, NewEvent("agent_ready", nil))
	next := receiveQueued(t, r, "not sent yet")
	finishQueue(resumed, next, "resumed answer")
	for _, state := range []string{"interrupted", "completed"} {
		select {
		case result := <-results:
			if result.State != state {
				t.Fatalf("result: %+v, want %s", result, state)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("missing recovered result")
		}
	}
}

func TestQueueRecoversCompletionWrittenBeforeParentCrash(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Root: root, RepoRoot: t.TempDir(), NewRunner: func(*Agent) Runner { return &queueRunner{input: make(chan string, 1)} }}
	mgr := New(cfg)
	a, err := mgr.Spawn(context.Background(), "initial")
	if err != nil {
		t.Fatal(err)
	}
	stopQueued(t, mgr, a)
	id, err := mgr.QueueTurn(a.ID, "completed before crash", nil)
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.followups[0].State = "dispatched"
	err = a.writeQueueLocked()
	a.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	log, err := OpenEventLog(a.EventLogPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Append(NewEvent("turn_end", map[string]any{"step": 2, "request_id": id, "response": "durable answer"})); err != nil {
		t.Fatal(err)
	}
	log.Close()
	restored := New(cfg)
	if n, errs := restored.Reload(); n != 1 || len(errs) != 0 {
		t.Fatalf("reload: %d %v", n, errs)
	}
	turns := restored.Get(a.ID).QueuedTurns()
	if len(turns) != 1 || turns[0].State != "completed" || turns[0].Response != "durable answer" {
		t.Fatalf("recovered result: %+v", turns)
	}
}

func TestQueueRejectsCorruptionAndFailedDurableWrites(t *testing.T) {
	cfg := Config{Root: t.TempDir(), RepoRoot: t.TempDir(), NewRunner: func(*Agent) Runner { return &queueRunner{input: make(chan string, 1)} }}
	mgr := New(cfg)
	a, err := mgr.Spawn(context.Background(), "initial")
	if err != nil {
		t.Fatal(err)
	}
	stopQueued(t, mgr, a)
	if err := os.WriteFile(a.queuePath(), []byte(`{"version":99}`), 0600); err != nil {
		t.Fatal(err)
	}
	restored := New(cfg)
	if n, errs := restored.Reload(); n != 1 || len(errs) != 1 {
		t.Fatalf("reload: %d %v", n, errs)
	}
	if _, err := restored.QueueTurn(a.ID, "must not overwrite corruption", nil); err == nil {
		t.Fatal("corrupt queue accepted work")
	}
	if _, err := restored.Resume(context.Background(), a.ID); err == nil {
		t.Fatal("corrupt queue resumed")
	}
	a.EventLogPath = filepath.Join(t.TempDir(), "missing", "events.jsonl")
	if _, err := mgr.QueueTurn(a.ID, "must be durable", nil); err == nil {
		t.Fatal("accepted without durable write")
	}
	if len(a.QueuedTurns()) != 0 {
		t.Fatal("failed write retained accepted work")
	}
}
