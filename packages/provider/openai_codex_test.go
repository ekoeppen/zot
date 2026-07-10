package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// An image-only tool result must not serialize to an empty
// function_call_output (the Responses API may reject it) and a
// following user-message image must serialize as input_image so the
// model actually receives the bytes.
func TestCodexImageToolResultMirror(t *testing.T) {
	c := NewOpenAICodex("token", "acct", "").(*codexClient)

	wire, err := c.buildRequest(Request{
		Model: "gpt-5.5",
		Messages: []Message{
			{Role: RoleUser, Content: []Content{TextBlock{Text: "look at this"}}},
			{Role: RoleAssistant, Content: []Content{
				ToolCallBlock{ID: "call_1", Name: "read", Arguments: []byte(`{"path":"x.png"}`)},
			}},
			{Role: RoleTool, Content: []Content{
				ToolResultBlock{CallID: "call_1", Content: []Content{
					ImageBlock{MimeType: "image/png", Data: []byte("png-bytes")},
				}},
			}},
			// The agent loop appends this mirror after an image tool result.
			{Role: RoleUser, Content: []Content{
				TextBlock{Text: "Tool output included the following image content:"},
				ImageBlock{MimeType: "image/png", Data: []byte("png-bytes")},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var sawFnOutput, sawInputImage bool
	for _, item := range wire.Input {
		switch v := item.(type) {
		case codexFunctionCallOutput:
			sawFnOutput = true
			if strings.TrimSpace(v.Output) == "" {
				t.Fatalf("image-only tool result produced empty function_call_output")
			}
			if !strings.Contains(strings.ToLower(v.Output), "image") {
				t.Fatalf("placeholder should mention image, got %q", v.Output)
			}
		case codexInputMessage:
			for _, ct := range v.Content {
				if img, ok := ct.(codexInputImage); ok && img.Type == "input_image" {
					sawInputImage = true
				}
			}
		}
	}
	if !sawFnOutput {
		t.Fatalf("no function_call_output emitted")
	}
	if !sawInputImage {
		t.Fatalf("mirrored user image was not serialized as input_image")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCodexPreviewModelUsesCodexCLIShape(t *testing.T) {
	c := NewOpenAICodex("token", "acct", "https://example.test/backend-api/codex/responses").(*codexClient)
	var gotReq *http.Request
	var gotBody codexRequest
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotReq = r
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	events, err := c.Stream(context.Background(), Request{
		Model:    "gpt-5.6-terra",
		Messages: []Message{{Role: RoleUser, Content: []Content{TextBlock{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}

	if gotReq == nil {
		t.Fatal("request was not sent")
	}
	if gotReq.Header.Get("originator") != "codex_cli_rs" {
		t.Fatalf("originator = %q", gotReq.Header.Get("originator"))
	}
	if gotReq.Header.Get("user-agent") != "codex_cli_rs/0.0.0" {
		t.Fatalf("user-agent = %q", gotReq.Header.Get("user-agent"))
	}
	if gotBody.PromptCacheKey == "" {
		t.Fatal("prompt_cache_key was not set")
	}
	if gotReq.Header.Get("session-id") != gotBody.PromptCacheKey {
		t.Fatalf("session-id = %q, prompt_cache_key = %q", gotReq.Header.Get("session-id"), gotBody.PromptCacheKey)
	}
}

func TestOpenAIGPT56DoesNotUseCodexCLIRouting(t *testing.T) {
	named := NewOpenAIResponsesNamed("token", "https://example.test/v1/responses", "openai").(*renamedClient)
	c := named.inner.(*codexClient)
	var gotReq *http.Request
	var gotBody codexRequest
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotReq = r
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	events, err := c.Stream(context.Background(), Request{
		Model:    "gpt-5.6-sol",
		Messages: []Message{{Role: RoleUser, Content: []Content{TextBlock{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}

	if gotReq == nil {
		t.Fatal("request was not sent")
	}
	if gotReq.Header.Get("session-id") != "" {
		t.Fatalf("session-id = %q", gotReq.Header.Get("session-id"))
	}
	if gotBody.PromptCacheKey != "" {
		t.Fatalf("prompt_cache_key = %q", gotBody.PromptCacheKey)
	}
}

func TestCodexNestedStreamError(t *testing.T) {
	c := NewOpenAICodex("token", "acct", "").(*codexClient)
	resp := &http.Response{
		Body: io.NopCloser(strings.NewReader("data: {\"type\":\"error\",\"error\":{\"code\":\"model_not_available\",\"message\":\"limited preview\"}}\n\n")),
	}
	out := make(chan Event, 16)
	go c.runStream(context.Background(), resp, Request{Model: "gpt-5.6-sol"}, out)

	var got error
	for ev := range out {
		if done, ok := ev.(EventDone); ok {
			got = done.Err
		}
	}
	if got == nil || got.Error() != "codex error: limited preview" {
		t.Fatalf("error = %v", got)
	}
}

func TestCodexSolKeepsZotShape(t *testing.T) {
	c := NewOpenAICodex("token", "acct", "https://example.test/backend-api/codex/responses").(*codexClient)
	var gotReq *http.Request
	var body bytes.Buffer
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotReq = r
		_, _ = body.ReadFrom(r.Body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	events, err := c.Stream(context.Background(), Request{
		Model:    "gpt-5.6-sol",
		Messages: []Message{{Role: RoleUser, Content: []Content{TextBlock{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}

	if gotReq == nil {
		t.Fatal("request was not sent")
	}
	if gotReq.Header.Get("originator") != "zot" {
		t.Fatalf("originator = %q", gotReq.Header.Get("originator"))
	}
	if gotReq.Header.Get("session-id") != "" {
		t.Fatalf("session-id = %q", gotReq.Header.Get("session-id"))
	}
	if strings.Contains(body.String(), "prompt_cache_key") {
		t.Fatalf("Sol request unexpectedly included prompt_cache_key: %s", body.String())
	}
}
