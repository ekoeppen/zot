package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/patriceckhart/zot/packages/agent/swarm"
)

func TestSwarmControlsEnforceScopeAndEnabled(t *testing.T) {
	mgr := swarm.New(swarm.Config{Root: t.TempDir(), RepoRoot: t.TempDir(), NewRunner: func(*swarm.Agent) swarm.Runner {
		return swarm.RunnerFunc(func(ctx context.Context, _ swarm.Sink) error { <-ctx.Done(); return ctx.Err() })
	}})
	mgr.SetActiveSession("other")
	a, err := mgr.Spawn(context.Background(), "private task")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mgr.Stop(a.ID); a.Wait() })
	mgr.SetActiveSession("current")
	enabled := true
	for _, tool := range SwarmControlTools(mgr, func() bool { return enabled }, nil) {
		if !json.Valid(tool.Schema()) {
			t.Fatalf("invalid schema for %s", tool.Name())
		}
		args, _ := json.Marshal(map[string]string{"agent_id": a.ID, "message": "follow up"})
		result, err := tool.Execute(context.Background(), args, nil)
		if err != nil || !result.IsError {
			t.Fatalf("%s allowed foreign agent: %+v %v", tool.Name(), result, err)
		}
		enabled = false
		result, err = tool.Execute(context.Background(), json.RawMessage(`{}`), nil)
		if err != nil || !result.IsError {
			t.Fatalf("%s allowed disabled access", tool.Name())
		}
		enabled = true
	}
	mgr.SetActiveSession("other")
	controls := SwarmControlTools(mgr, func() bool { return true }, nil)
	args, _ := json.Marshal(map[string]string{"agent_id": a.ID})
	result, err := controls[0].Execute(context.Background(), args, nil)
	if err != nil || result.IsError {
		t.Fatalf("status: %+v %v", result, err)
	}
	result, err = controls[1].Execute(context.Background(), args, nil)
	if err != nil || result.IsError {
		t.Fatalf("kill: %+v %v", result, err)
	}
	a.Wait()
	if a.Status() != swarm.StatusKilled {
		t.Fatalf("status %s", a.Status())
	}
}

func TestSwarmControlValidation(t *testing.T) {
	mgr := swarm.New(swarm.Config{Root: t.TempDir()})
	for _, tool := range SwarmControlTools(mgr, func() bool { return true }, nil) {
		result, err := tool.Execute(context.Background(), json.RawMessage(`{`), nil)
		if err != nil || !result.IsError {
			t.Fatalf("%s accepted malformed args", tool.Name())
		}
		result, err = tool.Execute(context.Background(), json.RawMessage(`{}`), nil)
		if err != nil {
			t.Fatal(err)
		}
		if (tool.Name() != "swarm_status") != result.IsError {
			t.Fatalf("%s wrong empty args result", tool.Name())
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := tool.Execute(ctx, json.RawMessage(`{}`), nil); err == nil {
			t.Fatalf("%s ignored cancellation", tool.Name())
		}
	}
}
