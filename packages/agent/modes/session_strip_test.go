package modes

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/patriceckhart/zot/packages/core"
	"github.com/patriceckhart/zot/packages/provider"
)

func TestSessionStripRemovesToolCallsAndPersistsCompaction(t *testing.T) {
	ag := core.NewAgent(nil, "claude-sonnet-4-5", "", nil)
	var persisted []provider.Message
	ag.OnTranscriptCompacted = func(msgs []provider.Message) {
		persisted = append([]provider.Message(nil), msgs...)
	}
	ag.SetMessages([]provider.Message{
		{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: "check repo"}}},
		{Role: provider.RoleAssistant, Content: []provider.Content{
			provider.TextBlock{Text: "Checking."},
			provider.ToolCallBlock{ID: "c1", Name: "bash", Arguments: json.RawMessage(`{"command":"ls"}`)},
		}},
		{Role: provider.RoleTool, Content: []provider.Content{
			provider.ToolResultBlock{CallID: "c1", Content: []provider.Content{provider.TextBlock{Text: "main.go"}}},
		}},
		{Role: provider.RoleAssistant, Content: []provider.Content{provider.TextBlock{Text: "Found main.go."}}},
	})

	i := NewInteractive(InteractiveConfig{Agent: ag})
	i.runSlash(context.Background(), "/session strip")

	if i.statusErr != "" {
		t.Fatalf("unexpected error: %q", i.statusErr)
	}
	if i.statusOK != "stripped 1 tool call and 1 tool response" {
		t.Fatalf("statusOK = %q", i.statusOK)
	}
	if len(ag.Messages()) != 3 || len(i.view.Messages) != 3 {
		t.Fatalf("transcript lengths = (%d, %d), want 3", len(ag.Messages()), len(i.view.Messages))
	}
	if len(persisted) != 3 {
		t.Fatalf("persisted checkpoint length = %d, want 3", len(persisted))
	}

	// Second strip is a no-op.
	persisted = nil
	i.runSlash(context.Background(), "/session strip")
	if i.statusOK != "no tool calls or responses to strip" {
		t.Fatalf("second strip statusOK = %q", i.statusOK)
	}
	if persisted != nil {
		t.Fatalf("second strip unexpectedly persisted checkpoint: %+v", persisted)
	}
}

func TestSessionStripDialogAndGuards(t *testing.T) {
	// Picker includes strip and executes it on selection.
	ag := core.NewAgent(nil, "claude-sonnet-4-5", "", nil)
	ag.SetMessages([]provider.Message{
		{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: "hi"}}},
		{Role: provider.RoleAssistant, Content: []provider.Content{
			provider.ToolCallBlock{ID: "c1", Name: "read", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: provider.RoleTool, Content: []provider.Content{
			provider.ToolResultBlock{CallID: "c1", Content: []provider.Content{provider.TextBlock{Text: "data"}}},
		}},
	})
	i := NewInteractive(InteractiveConfig{Agent: ag})
	i.openSessionOpsDialog()
	last := i.sessionOpsDialog.items[len(i.sessionOpsDialog.items)-1]
	if last.action != "strip" {
		t.Fatalf("last session op = %+v, want strip", last)
	}
	i.doSessionOp(last.action, "")
	if len(ag.Messages()) != 1 {
		t.Fatalf("len(ag.Messages()) = %d, want 1", len(ag.Messages()))
	}

	// Attached mode refuses transcript mutation.
	attached := NewInteractive(InteractiveConfig{
		Agent:        ag,
		PromptDriver: func(context.Context, *core.Agent, string, func(core.AgentEvent)) error { return nil },
	})
	attached.runSlash(context.Background(), "/session strip")
	if !strings.Contains(attached.statusErr, "unavailable while attached") {
		t.Fatalf("attached statusErr = %q", attached.statusErr)
	}

	// Nil agent reports login error.
	noAgent := NewInteractive(InteractiveConfig{})
	noAgent.runSlash(context.Background(), "/session strip")
	if noAgent.statusErr == "" {
		t.Fatal("expected error when agent is nil")
	}
}
