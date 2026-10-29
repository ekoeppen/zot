package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/patriceckhart/zot/packages/agent/swarm"
	"github.com/patriceckhart/zot/packages/core"
)

type controlRunner struct{ inputs chan string }

func (r *controlRunner) Run(ctx context.Context, _ swarm.Sink) error { <-ctx.Done(); return ctx.Err() }
func (r *controlRunner) SendInput(msg string) error                  { r.inputs <- msg; return nil }

func TestSwarmCancelAndResumePreservePendingFollowups(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	inputs := make(chan string, 4)
	mgr := swarm.New(swarm.Config{Root: t.TempDir(), RepoRoot: t.TempDir(), NewRunner: func(*swarm.Agent) swarm.Runner { return &controlRunner{inputs} }})
	mgr.SetActiveSession("session")
	a, err := mgr.Spawn(ctx, "initial")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mgr.Stop(a.ID); mgr.Get(a.ID).Wait() })
	controls := map[string]core.Tool{}
	for _, tool := range SwarmControlTools(mgr, func() bool { return true }, func(*swarm.Agent, swarm.QueuedTurn) bool { return true }) {
		controls[tool.Name()] = tool
	}
	args, _ := json.Marshal(map[string]string{"agent_id": a.ID, "message": "after resume"})
	execute := func(name string, wantError bool) {
		t.Helper()
		result, err := controls[name].Execute(ctx, args, nil)
		if err != nil || result.IsError != wantError {
			t.Fatalf("%s: %+v %v", name, result, err)
		}
	}
	execute("swarm_cancel", false)
	select {
	case msg := <-inputs:
		if msg != "cancel" {
			t.Fatal(msg)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	execute("swarm_kill", false)
	a.Wait()
	execute("swarm_cancel", true)
	execute("swarm_send", false)
	pending := a.QueuedTurns()
	if len(pending) != 1 || pending[0].State != "pending" {
		t.Fatalf("pending: %+v", pending)
	}
	execute("swarm_resume", false)
	resumed := mgr.Get(a.ID)
	if resumed == a || !resumed.Resuming {
		t.Fatal("did not resume in place")
	}
	turns := resumed.QueuedTurns()
	if len(turns) != 1 || turns[0].ID != pending[0].ID || turns[0].State != "pending" {
		t.Fatalf("resume lost pending input: %+v", turns)
	}
}
