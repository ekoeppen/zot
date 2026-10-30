package modes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/patriceckhart/zot/packages/core"
	"github.com/patriceckhart/zot/packages/provider"
)

func TestCompactionCountSuccessfulReplacements(t *testing.T) {
	for _, failure := range []error{nil, errors.New("failed"), context.Canceled, context.DeadlineExceeded} {
		client := &turnCompactionClient{compactErr: failure}
		agent := core.NewAgent(client, "test", "", nil)
		agent.SetMessages([]provider.Message{{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: "work"}}}})
		i := NewInteractive(InteractiveConfig{Agent: agent})
		for attempt := 1; attempt <= 2; attempt++ {
			_, err := i.compactTranscript(context.Background(), 0, nil)
			if !errors.Is(err, failure) {
				t.Fatalf("error = %v, want %v", err, failure)
			}
			want := 0
			if failure == nil {
				want = attempt
			}
			if i.compactionsSinceInput != want {
				t.Fatalf("count = %d, want %d", i.compactionsSinceInput, want)
			}
		}
	}
}

func TestCompactionCountUserBoundaries(t *testing.T) {
	i := NewInteractive(InteractiveConfig{})
	i.compactionsSinceInput = 2
	i.awaitingInitialUserMessage = true
	// A pre-turn compaction belongs to the pending prompt, whose initial
	// user_message event must not erase it when the prompt finally resumes.
	i.handleEvent(core.EvUserMessage{})
	if i.compactionsSinceInput != 2 {
		t.Fatal("initial user message erased pre-turn count")
	}
	i.queueFollowUp("queued", nil)
	if i.compactionsSinceInput != 2 {
		t.Fatal("queue submission erased count before execution")
	}
	i.handleEvent(core.EvTurnStart{Step: 2})
	i.handleEvent(core.EvDone{})
	if i.compactionsSinceInput != 2 {
		t.Fatal("continuation or completion erased count")
	}
	i.handleEvent(core.EvUserMessage{})
	if i.compactionsSinceInput != 0 {
		t.Fatal("executed queued message did not reset count")
	}
}

func TestCompactionCountPreTurnAndNextRequest(t *testing.T) {
	client := &turnCompactionClient{}
	agent := core.NewAgent(client, "test", "", nil)
	messages := make([]provider.Message, 6)
	for idx := range messages {
		messages[idx] = provider.Message{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: "history"}}}
	}
	agent.SetMessages(messages)
	i := NewInteractive(InteractiveConfig{Agent: agent, Provider: "anthropic", Model: "claude-sonnet-4-5"})
	i.compactionsSinceInput = 7
	i.lastCtxInput = 180000
	i.startTurn(context.Background(), "new request")
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(time.Millisecond)
	defer poll.Stop()
	for {
		i.mu.Lock()
		done := !i.busy && !i.awaitingInitialUserMessage
		count := i.compactionsSinceInput
		i.mu.Unlock()
		if done {
			if count != 1 {
				t.Fatalf("pre-turn count = %d, want 1", count)
			}
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("pending prompt did not finish")
		case <-poll.C:
		}
	}
	i.startTurn(context.Background(), "next request")
	waitTurnCompactionIdle(t, i)
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.compactionsSinceInput != 0 {
		t.Fatal("next request retained count")
	}
}

func TestCompactionCountClear(t *testing.T) {
	i := NewInteractive(InteractiveConfig{})
	i.compactionsSinceInput = 3
	i.awaitingInitialUserMessage = true
	i.runSlash(context.Background(), "/clear")
	if i.compactionsSinceInput != 0 || i.awaitingInitialUserMessage {
		t.Fatal("clear retained compaction state")
	}
}

func TestCompactionCountBetweenToolBatches(t *testing.T) {
	client := &turnCompactionClient{}
	agent := core.NewAgent(client, "test", "", core.NewRegistry(&shortcutTool{}))
	i := NewInteractive(InteractiveConfig{Agent: agent, Provider: "anthropic", Model: "claude-sonnet-4-5"})
	i.startTurn(context.Background(), "work")
	waitTurnCompactionIdle(t, i)
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.compactionsSinceInput != 1 {
		t.Fatalf("between-batch count = %d, want 1", i.compactionsSinceInput)
	}
}

func TestCompactionCountNewSession(t *testing.T) {
	i := NewInteractive(InteractiveConfig{NewSession: func(string, string) error { return nil }})
	i.compactionsSinceInput = 3
	i.awaitingInitialUserMessage = true
	i.runSlash(context.Background(), "/new")
	if i.compactionsSinceInput != 0 || i.awaitingInitialUserMessage {
		t.Fatal("new session retained compaction state")
	}
}

func TestCompactionCountNoAgent(t *testing.T) {
	i := NewInteractive(InteractiveConfig{})
	i.compactionsSinceInput = 3
	i.startTurn(context.Background(), "cannot execute")
	if i.compactionsSinceInput != 3 {
		t.Fatal("unexecuted request erased count")
	}
}
