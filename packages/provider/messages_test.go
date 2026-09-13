package provider

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestUnmarshalMessageAndMessages(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	tests := []struct {
		name     string
		input    string
		expected Message
	}{
		{
			name:  "string content user message",
			input: `{"role":"user","content":"hello world"}`,
			expected: Message{
				Role:    RoleUser,
				Content: []Content{TextBlock{Text: "hello world"}},
			},
		},
		{
			name:  "array content with text and image",
			input: `{"role":"user","content":[{"text":"look at this"},{"mime_type":"image/png","data":"aGVsbG8="}]}`,
			expected: Message{
				Role: RoleUser,
				Content: []Content{
					TextBlock{Text: "look at this"},
					ImageBlock{MimeType: "image/png", Data: []byte("hello")},
				},
			},
		},
		{
			name:  "tool call message",
			input: `{"role":"assistant","content":[{"id":"call-1","name":"bash","arguments":{"command":"ls"}}]}`,
			expected: Message{
				Role: RoleAssistant,
				Content: []Content{
					ToolCallBlock{
						ID:        "call-1",
						Name:      "bash",
						Arguments: json.RawMessage(`{"command":"ls"}`),
					},
				},
			},
		},
		{
			name:  "tool result message",
			input: `{"role":"tool","content":[{"call_id":"call-1","content":[{"text":"file.txt"}],"is_error":false}]}`,
			expected: Message{
				Role: RoleTool,
				Content: []Content{
					ToolResultBlock{
						CallID:  "call-1",
						Content: []Content{TextBlock{Text: "file.txt"}},
						IsError: false,
					},
				},
			},
		},
		{
			name:  "reasoning block message",
			input: `{"role":"assistant","content":[{"reasoning_id":"r-1","summary":"thinking...","encrypted_content":"enc"}]}`,
			expected: Message{
				Role: RoleAssistant,
				Content: []Content{
					ReasoningBlock{
						ID:        "r-1",
						Summary:   "thinking...",
						Encrypted: "enc",
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := UnmarshalMessage([]byte(tc.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Role != tc.expected.Role {
				t.Errorf("role = %q, want %q", got.Role, tc.expected.Role)
			}
			if len(got.Content) != len(tc.expected.Content) {
				t.Fatalf("content len = %d, want %d", len(got.Content), len(tc.expected.Content))
			}
			for i := range got.Content {
				if !reflect.DeepEqual(got.Content[i], tc.expected.Content[i]) {
					t.Errorf("content[%d] = %#v, want %#v", i, got.Content[i], tc.expected.Content[i])
				}
			}
		})
	}

	// Test UnmarshalMessages
	msgsJSON := `[
		{"role":"user","content":"hi"},
		{"role":"assistant","content":[{"text":"hello"}]}
	]`
	msgs, err := UnmarshalMessages([]byte(msgsJSON))
	if err != nil {
		t.Fatalf("UnmarshalMessages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	_ = now
}
