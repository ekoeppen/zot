package modes

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/patriceckhart/zot/packages/agent/swarm"
	"github.com/patriceckhart/zot/packages/core"
)

func TestSwarmControlToggleRevokesCapturedTools(t *testing.T) {
	iv := &Interactive{agent: core.NewAgent(nil, "test", "", core.Registry{})}
	iv.cfg.Swarm = swarm.New(swarm.Config{Root: t.TempDir()})
	iv.applyAutoSwarmTool(true)
	registered := iv.agent.ToolRegistry()
	for _, name := range []string{"swarm_spawn", "swarm_status", "swarm_send", "swarm_kill", "swarm_cancel", "swarm_resume"} {
		if registered[name] == nil {
			t.Fatalf("missing %s", name)
		}
	}
	iv.applyAutoSwarmTool(false)
	for name, tool := range registered {
		if iv.agent.ToolRegistry()[name] != nil {
			t.Fatalf("tool still registered: %s", name)
		}
		result, err := tool.Execute(context.Background(), json.RawMessage(`{}`), nil)
		if err != nil || !result.IsError {
			t.Fatalf("captured tool %s remains enabled", name)
		}
	}
}

func TestSwarmFollowupSummaryUsesFollowupTaskAndOutcome(t *testing.T) {
	mgr := swarm.New(swarm.Config{Root: t.TempDir(), RepoRoot: t.TempDir(), NewRunner: func(*swarm.Agent) swarm.Runner {
		return swarm.RunnerFunc(func(_ context.Context, sink swarm.Sink) error { sink.Transcript("initial answer"); return nil })
	}})
	a, err := mgr.Spawn(context.Background(), "initial task")
	if err != nil {
		t.Fatal(err)
	}
	a.Wait()
	iv := newQueuedAutoSwarmInteractive()
	iv.SwarmQueuedTurnCompleted(a, swarm.QueuedTurn{ID: "request-1", Text: "follow-up task", State: "failed", Error: "delivery failed"})
	update := waitForQueuedPrompt(t, iv)
	for _, want := range []string{"status: failed", "follow-up task", "delivery failed"} {
		if !strings.Contains(update, want) {
			t.Fatalf("missing %q in %s", want, update)
		}
	}
	if strings.Contains(update, "initial answer") {
		t.Fatalf("old response attributed to failed follow-up: %s", update)
	}
}
