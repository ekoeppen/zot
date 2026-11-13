package core

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/patriceckhart/zot/packages/provider"
)

func TestStripToolCallsRemovesToolBlocksMessagesAndMirrors(t *testing.T) {
	input := []provider.Message{
		{
			Role:    provider.RoleUser,
			Content: []provider.Content{provider.TextBlock{Text: "check the files"}},
		},
		{
			Role: provider.RoleAssistant,
			Content: []provider.Content{
				provider.ReasoningBlock{ID: "r1", Summary: "thinking about bash"},
				provider.TextBlock{Text: "Let me check."},
				provider.ToolCallBlock{ID: "call-1", Name: "bash", Arguments: json.RawMessage(`{"command":"ls"}`)},
			},
			Meta: map[string]string{
				provider.NestedToolCallsMetaKey: `{"call-1":[]}`,
				"keep":                          "yes",
			},
		},
		{
			Role: provider.RoleTool,
			Content: []provider.Content{
				provider.ToolResultBlock{
					CallID:  "call-1",
					Content: []provider.Content{provider.TextBlock{Text: "a.go"}},
				},
			},
			AddedToolNames: []string{"lookup_weather"},
			Meta:           map[string]string{"tool_state:codemode": `{"value":1}`},
		},
		{
			Role: provider.RoleUser,
			Content: []provider.Content{
				provider.TextBlock{Text: "Tool output included the following image content:"},
				provider.ImageBlock{MimeType: "image/png", Data: []byte("png")},
			},
		},
		{
			Role: provider.RoleAssistant,
			Content: []provider.Content{
				provider.ReasoningBlock{ID: "r2", Summary: "tool-only reasoning"},
				provider.TextBlock{Text: "   "},
				provider.ToolCallBlock{ID: "call-2", Name: "read", Arguments: json.RawMessage(`{"path":"a.go"}`)},
			},
		},
		{
			Role: provider.RoleTool,
			Content: []provider.Content{
				provider.ToolResultBlock{
					CallID:  "call-2",
					Content: []provider.Content{provider.TextBlock{Text: "package main"}},
				},
			},
			AddedToolNames: []string{"lookup_weather", "extra_tool"},
			Meta:           map[string]string{"tool_state:codemode": `{"value":2}`},
		},
		{
			Role:    provider.RoleAssistant,
			Content: []provider.Content{provider.TextBlock{Text: "Done checking."}},
		},
	}

	got, calls, results := StripToolCalls(input)
	if calls != 2 || results != 2 {
		t.Fatalf("removed = (%d calls, %d results), want (2, 2)", calls, results)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3: %+v", len(got), got)
	}
	if got[0].Role != provider.RoleUser || got[0].Content[0].(provider.TextBlock).Text != "check the files" {
		t.Fatalf("got[0] = %+v", got[0])
	}
	if !reflect.DeepEqual(got[0].AddedToolNames, []string{"lookup_weather", "extra_tool"}) {
		t.Fatalf("AddedToolNames = %+v", got[0].AddedToolNames)
	}
	if got[1].Role != provider.RoleAssistant || len(got[1].Content) != 2 {
		t.Fatalf("got[1] = %+v", got[1])
	}
	if _, ok := got[1].Content[0].(provider.ReasoningBlock); !ok {
		t.Fatalf("got[1].Content[0] = %T, want ReasoningBlock", got[1].Content[0])
	}
	if tb, ok := got[1].Content[1].(provider.TextBlock); !ok || tb.Text != "Let me check." {
		t.Fatalf("got[1].Content[1] = %+v", got[1].Content[1])
	}
	if _, ok := got[1].Meta[provider.NestedToolCallsMetaKey]; ok {
		t.Fatalf("nested tool metadata was not stripped: %+v", got[1].Meta)
	}
	if got[1].Meta["keep"] != "yes" {
		t.Fatalf("unrelated meta lost: %+v", got[1].Meta)
	}
	if _, ok := input[1].Meta[provider.NestedToolCallsMetaKey]; !ok {
		t.Fatal("input meta mutated")
	}
	if got[2].Role != provider.RoleAssistant || got[2].Content[0].(provider.TextBlock).Text != "Done checking." {
		t.Fatalf("got[2] = %+v", got[2])
	}
	if gotState := string(readToolState(got)["codemode"]); gotState != `{"value":2}` {
		t.Fatalf("preserved tool_state = %q, want {\"value\":2}", gotState)
	}
}

func TestStripToolCallsDropsLeadingAssistantBeforeFirstUser(t *testing.T) {
	// Simulates a >100 message session resumed mid-turn via trimMessagesForResume.
	input := []provider.Message{
		{Role: provider.RoleAssistant, Content: []provider.Content{
			provider.ToolCallBlock{ID: "c1", Name: "bash", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: provider.RoleTool, Content: []provider.Content{
			provider.ToolResultBlock{CallID: "c1", Content: []provider.Content{provider.TextBlock{Text: "out"}}},
		}},
		{Role: provider.RoleAssistant, Content: []provider.Content{provider.TextBlock{Text: "mid-turn reply"}}},
		{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: "next question"}}},
		{Role: provider.RoleAssistant, Content: []provider.Content{provider.TextBlock{Text: "answer"}}},
	}

	got, calls, results := StripToolCalls(input)
	if calls != 1 || results != 1 {
		t.Fatalf("removed = (%d, %d), want (1, 1)", calls, results)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2: %+v", len(got), got)
	}
	if got[0].Role != provider.RoleUser || got[0].Content[0].(provider.TextBlock).Text != "next question" {
		t.Fatalf("got[0] = %+v, want user message", got[0])
	}
	if got[1].Role != provider.RoleAssistant || got[1].Content[0].(provider.TextBlock).Text != "answer" {
		t.Fatalf("got[1] = %+v, want assistant answer", got[1])
	}

	// Without tool calls/results, StripToolCalls is a no-op.
	noTools := []provider.Message{
		{Role: provider.RoleAssistant, Content: []provider.Content{provider.TextBlock{Text: "orphan"}}},
		{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: "hi"}}},
	}
	unchanged, c2, r2 := StripToolCalls(noTools)
	if c2 != 0 || r2 != 0 || len(unchanged) != 2 {
		t.Fatalf("unexpected change on no-tool input: (%d, %d, %d)", c2, r2, len(unchanged))
	}
}

func TestAgentStripToolCallsPersistsCompactionCheckpointAndSkipsNoop(t *testing.T) {
	dir := t.TempDir()
	sessPath := filepath.Join(dir, "session.jsonl")
	sess, err := NewSessionAtPath(sessPath, dir, "anthropic", "claude-sonnet-4-5", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	ag := NewAgent(nil, "claude-sonnet-4-5", "", nil)
	var persistedCalls int
	ag.OnTranscriptCompacted = func(messages []provider.Message) {
		persistedCalls++
		if err := sess.AppendCompaction(messages); err != nil {
			t.Fatal(err)
		}
	}

	plain := []provider.Message{
		{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: "hi"}}},
		{Role: provider.RoleAssistant, Content: []provider.Content{provider.TextBlock{Text: "hello"}}},
	}
	ag.SetMessages(plain)
	calls, results := ag.StripToolCalls()
	if calls != 0 || results != 0 || persistedCalls != 0 {
		t.Fatalf("no-op strip = (%d, %d, %d), want (0, 0, 0)", calls, results, persistedCalls)
	}

	withTools := []provider.Message{
		{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: "run ls"}}},
		{Role: provider.RoleAssistant, Content: []provider.Content{
			provider.ToolCallBlock{ID: "c1", Name: "bash", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: provider.RoleTool, Content: []provider.Content{
			provider.ToolResultBlock{CallID: "c1", Content: []provider.Content{provider.TextBlock{Text: "ok"}}},
		}, AddedToolNames: []string{"deferred_tool"}, Meta: map[string]string{"tool_state:codemode": `{"ok":true}`}},
		{Role: provider.RoleAssistant, Content: []provider.Content{provider.TextBlock{Text: "all set"}}},
	}
	for _, m := range withTools {
		if err := sess.AppendMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	ag.SetMessages(withTools)
	revBefore := ag.Revision()

	calls, results = ag.StripToolCalls()
	if calls != 1 || results != 1 || persistedCalls != 1 {
		t.Fatalf("strip = (%d, %d, %d), want (1, 1, 1)", calls, results, persistedCalls)
	}
	if ag.Revision() == revBefore {
		t.Fatal("agent revision did not advance")
	}
	if len(ag.Messages()) != 2 {
		t.Fatalf("in-memory messages = %d, want 2", len(ag.Messages()))
	}

	_, loaded, err := ReadSession(sessPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("reloaded messages = %d, want 2: %+v", len(loaded), loaded)
	}
	if loaded[0].Role != provider.RoleUser || loaded[1].Role != provider.RoleAssistant {
		t.Fatalf("reloaded roles = (%s, %s)", loaded[0].Role, loaded[1].Role)
	}
	if !reflect.DeepEqual(loaded[0].AddedToolNames, []string{"deferred_tool"}) {
		t.Fatalf("reloaded AddedToolNames = %+v", loaded[0].AddedToolNames)
	}
	if string(readToolState(loaded)["codemode"]) != `{"ok":true}` {
		t.Fatalf("reloaded tool_state = %+v", readToolState(loaded))
	}
}
