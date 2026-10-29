package modes

import (
	"strings"
	"testing"

	"github.com/patriceckhart/zot/packages/agent/swarm"
)

func TestAutoSwarmReportsFinishedTaskWhileSiblingRemainsActive(t *testing.T) {
	iv := newQueuedAutoSwarmInteractive()
	finished := &swarmWatchEntry{agent: &swarm.Agent{ID: "finished-agent", Task: "finished task"}}
	blocked := &swarmWatchEntry{agent: &swarm.Agent{ID: "blocked-agent", Task: "blocked task"}}
	iv.swarmWatch = []*swarmWatchEntry{finished, blocked}
	iv.completeSwarmWatchEntry(finished, "completed", "")
	update := waitForQueuedPrompt(t, iv)
	if !strings.Contains(update, "finished-agent") || strings.Contains(update, "blocked-agent") {
		t.Fatalf("wrong progress report: %s", update)
	}
	iv.swarmWatchMu.Lock()
	remaining := len(iv.swarmWatch)
	iv.swarmWatchMu.Unlock()
	if remaining != 1 {
		t.Fatalf("remaining watchers: %d", remaining)
	}
	iv.completeSwarmWatchEntry(finished, "failed", "duplicate")
	iv.completeSwarmWatchEntry(blocked, "failed", "network error")
	iv.mu.Lock()
	count := len(iv.queued)
	iv.mu.Unlock()
	if count != 2 {
		t.Fatalf("updates: %d, want one per task", count)
	}
}

func TestAutoSwarmInitialResultDoesNotLeakIntoAnotherSession(t *testing.T) {
	iv := newQueuedAutoSwarmInteractive()
	iv.cfg.Swarm = swarm.New(swarm.Config{Root: t.TempDir()})
	iv.cfg.Swarm.SetActiveSession("different")
	entry := &swarmWatchEntry{agent: &swarm.Agent{ID: "agent", SessionID: "original"}}
	iv.swarmWatch = []*swarmWatchEntry{entry}
	iv.completeSwarmWatchEntry(entry, "completed", "")
	iv.mu.Lock()
	defer iv.mu.Unlock()
	if len(iv.queued) != 0 {
		t.Fatal("reported initial result into another session")
	}
}

func TestSwarmQueuedNoticeWaitsForItsSessionAndUsesRecordedResponse(t *testing.T) {
	iv := newQueuedAutoSwarmInteractive()
	iv.cfg.Swarm = swarm.New(swarm.Config{Root: t.TempDir()})
	a := &swarm.Agent{ID: "agent", SessionID: "original"}
	turn := swarm.QueuedTurn{ID: "request", Text: "follow-up", State: "completed", Response: "recorded answer"}
	iv.cfg.Swarm.SetActiveSession("different")
	if iv.SwarmQueuedTurnCompleted(a, turn) {
		t.Fatal("reported into another session")
	}
	iv.cfg.Swarm.SetActiveSession("original")
	if !iv.SwarmQueuedTurnCompleted(a, turn) {
		t.Fatal("did not report into original session")
	}
	update := waitForQueuedPrompt(t, iv)
	for _, want := range []string{"recorded answer", "request: request", "follow-up"} {
		if !strings.Contains(update, want) {
			t.Fatalf("missing %q in %s", want, update)
		}
	}
}
