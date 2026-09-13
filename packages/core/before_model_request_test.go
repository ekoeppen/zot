package core

import (
	"context"
	"testing"

	"github.com/patriceckhart/zot/packages/provider"
)

type capturingClient struct {
	lastReq provider.Request
}

func (c *capturingClient) Name() string { return "capturing" }

func (c *capturingClient) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	c.lastReq = req
	out := make(chan provider.Event, 2)
	go func() {
		defer close(out)
		out <- provider.EventStart{Provider: "capturing", Model: req.Model}
		out <- provider.EventDone{
			Stop: provider.StopEnd,
			Message: provider.Message{
				Role:    provider.RoleAssistant,
				Content: []provider.Content{provider.TextBlock{Text: "done"}},
			},
		}
	}()
	return out, nil
}

func TestBeforeModelRequestTransform(t *testing.T) {
	client := &capturingClient{}
	a := NewAgent(client, "test-model", "test-system", Registry{})

	// Add 3 user/assistant turns
	a.SetMessages([]provider.Message{
		{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: "turn 1"}}},
		{Role: provider.RoleAssistant, Content: []provider.Content{provider.TextBlock{Text: "reply 1"}}},
		{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: "turn 2"}}},
	})

	hookCalled := false
	a.BeforeModelRequest = func(ctx context.Context, msgs []provider.Message) []provider.Message {
		hookCalled = true
		if len(msgs) != 3 {
			t.Fatalf("expected 3 input messages, got %d", len(msgs))
		}
		// Return only the last message with a dynamic prefix
		return []provider.Message{
			{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: "[transformed] turn 2"}}},
		}
	}

	if err := a.Continue(context.Background(), func(AgentEvent) {}); err != nil {
		t.Fatalf("Continue failed: %v", err)
	}

	if !hookCalled {
		t.Fatal("BeforeModelRequest hook was not called")
	}

	// Verify what the provider client received
	if len(client.lastReq.Messages) != 1 {
		t.Fatalf("client received %d messages, want 1", len(client.lastReq.Messages))
	}
	tb, ok := client.lastReq.Messages[0].Content[0].(provider.TextBlock)
	if !ok || tb.Text != "[transformed] turn 2" {
		t.Fatalf("client received %+v, want '[transformed] turn 2'", client.lastReq.Messages[0].Content)
	}

	// Verify underlying ground truth transcript has the original 3 messages + 1 new assistant reply (total 4)
	stored := a.Messages()
	if len(stored) != 4 {
		t.Fatalf("stored transcript has %d messages, want 4", len(stored))
	}
	t1, _ := stored[0].Content[0].(provider.TextBlock)
	if t1.Text != "turn 1" {
		t.Errorf("stored[0] = %q, want 'turn 1'", t1.Text)
	}
}
