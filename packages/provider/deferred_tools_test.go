package provider

import (
	"encoding/json"
	"testing"
)

func TestKimiK3DeferredToolsLoadAtToolResult(t *testing.T) {
	client := NewMoonshot("token", "").(*openaiClient)
	tools := []Tool{
		{Name: "search_tools", Schema: json.RawMessage(`{"type":"object"}`)},
		{Name: "lookup_weather", Schema: json.RawMessage(`{"type":"object"}`), Deferred: true},
	}
	wire, err := client.buildRequest(Request{
		Model: "kimi-k3",
		Tools: tools,
		Messages: []Message{
			{Role: RoleUser, Content: []Content{TextBlock{Text: "weather"}}},
			{Role: RoleAssistant, Content: []Content{ToolCallBlock{ID: "call-1", Name: "search_tools", Arguments: json.RawMessage(`{}`)}}},
			{Role: RoleTool, Content: []Content{ToolResultBlock{CallID: "call-1", Content: []Content{TextBlock{Text: "found"}}}}, AddedToolNames: []string{"lookup_weather"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(wire.Tools) != 1 || wire.Tools[0].Function.Name != "search_tools" {
		t.Fatalf("top-level tools = %+v, want only search_tools", wire.Tools)
	}
	var loaded []oaiTool
	for _, message := range wire.Messages {
		if message.Role == "system" && len(message.Tools) > 0 {
			loaded = message.Tools
		}
	}
	if len(loaded) != 1 || loaded[0].Function.Name != "lookup_weather" {
		t.Fatalf("loaded tools = %+v, want lookup_weather", loaded)
	}
}

func TestDeferredToolsFallbackToActiveTopLevelDefinitions(t *testing.T) {
	client := NewOpenAI("token", "").(*openaiClient)
	tools := []Tool{
		{Name: "base", Schema: json.RawMessage(`{"type":"object"}`)},
		{Name: "late", Schema: json.RawMessage(`{"type":"object"}`), Deferred: true},
	}
	wire, err := client.buildRequest(Request{Model: "gpt-5", Tools: tools})
	if err != nil {
		t.Fatal(err)
	}
	if len(wire.Tools) != 1 || wire.Tools[0].Function.Name != "base" {
		t.Fatalf("initial tools = %+v", wire.Tools)
	}

	wire, err = client.buildRequest(Request{
		Model:    "gpt-5",
		Tools:    tools,
		Messages: []Message{{Role: RoleTool, AddedToolNames: []string{"late"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(wire.Tools) != 2 {
		t.Fatalf("activated tools = %+v, want both definitions", wire.Tools)
	}
}

func TestKimiK3AndGrok45CatalogMetadata(t *testing.T) {
	for _, tc := range []struct {
		provider string
		model    string
	}{
		{"kimi", "k3"},
		{"moonshotai", "kimi-k3"},
		{"moonshotai-cn", "kimi-k3"},
		{"openrouter", "moonshotai/kimi-k3"},
		{"vercel-ai-gateway", "moonshotai/kimi-k3"},
	} {
		m, err := FindModel(tc.provider, tc.model)
		if err != nil {
			t.Fatalf("%s/%s: %v", tc.provider, tc.model, err)
		}
		if m.MaxOutput != 131072 || !m.Reasoning {
			t.Fatalf("%s/%s metadata = %+v", tc.provider, tc.model, m)
		}
	}
	m, err := FindModel("xai", "grok-4.5")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Reasoning || m.MaxOutput != 30000 {
		t.Fatalf("xai/grok-4.5 metadata = %+v", m)
	}
}
