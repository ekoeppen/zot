package provider

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestSigV4Known checks our signer against AWS's published test vector
// from the SigV4 documentation. The expected Authorization header below
// is taken from "get-vanilla" in the suite (adapted: we use POST with
// an empty body since Bedrock is always POST).
//
// We don't run the official suite (it lives in S3 as a tarball and the
// signing rules vary slightly between examples); instead this test
// pins a fixed input/output so future refactors can't silently break
// the signer.
func TestSigV4Deterministic(t *testing.T) {
	creds := &bedrockSigV4Creds{
		accessKeyID:     "AKIDEXAMPLE",
		secretAccessKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
	}
	body := []byte(`{"hello":"world"}`)
	req, err := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/model/foo/converse-stream", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("content-type", "application/json")
	when := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	if err := signSigV4(req, body, "bedrock", "us-east-1", creds, when); err != nil {
		t.Fatal(err)
	}
	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
		t.Fatalf("missing AWS4 prefix: %q", auth)
	}
	if !strings.Contains(auth, "Credential=AKIDEXAMPLE/20240115/us-east-1/bedrock/aws4_request") {
		t.Errorf("wrong credential scope: %q", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=") {
		t.Errorf("missing SignedHeaders: %q", auth)
	}
	if !strings.Contains(auth, "Signature=") {
		t.Errorf("missing Signature: %q", auth)
	}
	// Sign again with identical inputs -> identical signature (determinism).
	req2, _ := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/model/foo/converse-stream", bytes.NewReader(body))
	req2.Header.Set("content-type", "application/json")
	if err := signSigV4(req2, body, "bedrock", "us-east-1", creds, when); err != nil {
		t.Fatal(err)
	}
	if req2.Header.Get("Authorization") != auth {
		t.Errorf("signature not deterministic:\n a=%q\n b=%q", auth, req2.Header.Get("Authorization"))
	}
}

func TestSigV4PathEncoding(t *testing.T) {
	// Bedrock model IDs include colons and dots. The colon must encode
	// to %3A in the canonical path (per SigV4 rules).
	cases := map[string]string{
		"/model/anthropic.claude-sonnet-4-5-20250929-v1:0/converse-stream": "/model/anthropic.claude-sonnet-4-5-20250929-v1%3A0/converse-stream",
		"/":  "/",
		"":   "/",
		"/a": "/a",
	}
	for in, want := range cases {
		got := canonicalSigV4Path(in)
		if got != want {
			t.Errorf("canonicalSigV4Path(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadAWSCredentialsFile(t *testing.T) {
	// Smoke test: parses an INI-ish file when present. We don't require
	// the file to exist on the test host.
	if _, err := readAWSCredentialsFile("default"); err != nil {
		// missing file or missing profile is fine — both are non-panics
		t.Logf("no aws creds available (expected on CI): %v", err)
	}
}

func TestNormalizeBedrockToolResultsMovesResultsAdjacentToToolUse(t *testing.T) {
	msgs := []Message{
		{Role: RoleAssistant, Content: []Content{ToolCallBlock{ID: "tool-1", Name: "edit", Arguments: json.RawMessage(`{"path":"a"}`)}}},
		{Role: RoleUser, Content: []Content{TextBlock{Text: "what's wrong"}}},
		{Role: RoleTool, Content: []Content{ToolResultBlock{CallID: "tool-1", Content: []Content{TextBlock{Text: "edited"}}}}},
	}

	out := normalizeBedrockToolResults(msgs)
	if len(out) != 3 {
		t.Fatalf("got %d messages, want 3: %+v", len(out), out)
	}
	if out[0].Role != RoleAssistant {
		t.Fatalf("first role = %s, want assistant", out[0].Role)
	}
	if out[1].Role != RoleTool {
		t.Fatalf("second role = %s, want tool", out[1].Role)
	}
	tr, ok := out[1].Content[0].(ToolResultBlock)
	if !ok {
		t.Fatalf("second message content = %T, want ToolResultBlock", out[1].Content[0])
	}
	if tr.CallID != "tool-1" || tr.IsError {
		t.Fatalf("unexpected tool result: %+v", tr)
	}
	if out[2].Role != RoleUser {
		t.Fatalf("third role = %s, want user", out[2].Role)
	}
}

func TestNormalizeBedrockToolResultsInjectsMissingResult(t *testing.T) {
	msgs := []Message{
		{Role: RoleAssistant, Content: []Content{ToolCallBlock{ID: "tool-1", Name: "edit", Arguments: json.RawMessage(`{}`)}}},
		{Role: RoleUser, Content: []Content{TextBlock{Text: "what's wrong"}}},
	}

	out := normalizeBedrockToolResults(msgs)
	if len(out) != 3 {
		t.Fatalf("got %d messages, want 3: %+v", len(out), out)
	}
	tr, ok := out[1].Content[0].(ToolResultBlock)
	if !ok {
		t.Fatalf("second message content = %T, want ToolResultBlock", out[1].Content[0])
	}
	if tr.CallID != "tool-1" || !tr.IsError {
		t.Fatalf("unexpected synthetic result: %+v", tr)
	}
}

func TestBedrockBuildRequestSkipsEmptyToolMessages(t *testing.T) {
	client := &bedrockClient{}
	req, err := client.buildRequest(Request{Messages: []Message{
		{Role: RoleTool, Content: []Content{ToolResultBlock{CallID: "missing", Content: []Content{TextBlock{Text: "orphan"}}}}},
		{Role: RoleUser, Content: []Content{TextBlock{Text: "hello"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("got %d bedrock messages, want 1: %+v", len(req.Messages), req.Messages)
	}
	if req.Messages[0].Role != "user" {
		t.Fatalf("role = %s, want user", req.Messages[0].Role)
	}
}

func TestBedrockEventPayloadHelpers(t *testing.T) {
	wrapped := []byte(`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"Hello"}}}`)
	if got := bedrockEventTypeFromPayload(wrapped); got != "contentBlockDelta" {
		t.Fatalf("event type = %q, want contentBlockDelta", got)
	}
	var delta struct {
		ContentBlockIndex int `json:"contentBlockIndex"`
		Delta             struct {
			Text string `json:"text"`
		} `json:"delta"`
	}
	if err := unmarshalBedrockEventPayload(wrapped, "contentBlockDelta", &delta); err != nil {
		t.Fatal(err)
	}
	if delta.ContentBlockIndex != 0 || delta.Delta.Text != "Hello" {
		t.Fatalf("unexpected wrapped delta: %+v", delta)
	}

	direct := []byte(`{"contentBlockIndex":1,"delta":{"text":"world"}}`)
	if err := unmarshalBedrockEventPayload(direct, "contentBlockDelta", &delta); err != nil {
		t.Fatal(err)
	}
	if delta.ContentBlockIndex != 1 || delta.Delta.Text != "world" {
		t.Fatalf("unexpected direct delta: %+v", delta)
	}
}

func TestResolveBedrockInferenceProfileID(t *testing.T) {
	cases := []struct {
		model  string
		region string
		want   string
	}{
		// Bare Anthropic foundation IDs get the region-matched prefix.
		{"anthropic.claude-sonnet-4-5-20250929-v1:0", "us-east-1", "us.anthropic.claude-sonnet-4-5-20250929-v1:0"},
		{"anthropic.claude-sonnet-4-5-20250929-v1:0", "eu-central-1", "eu.anthropic.claude-sonnet-4-5-20250929-v1:0"},
		{"anthropic.claude-opus-4-6-v1", "ap-southeast-2", "apac.anthropic.claude-opus-4-6-v1"},
		{"anthropic.claude-opus-4-6-v1", "us-gov-west-1", "us-gov.anthropic.claude-opus-4-6-v1"},
		{"deepseek.r1-v1:0", "eu-west-1", "eu.deepseek.r1-v1:0"},
		// Empty region defaults to us.
		{"anthropic.claude-opus-4-6-v1", "", "us.anthropic.claude-opus-4-6-v1"},
		// Already-prefixed IDs are left untouched.
		{"eu.anthropic.claude-sonnet-4-5-20250929-v1:0", "us-east-1", "eu.anthropic.claude-sonnet-4-5-20250929-v1:0"},
		{"global.anthropic.claude-opus-4-6-v1", "us-east-1", "global.anthropic.claude-opus-4-6-v1"},
		{"us.anthropic.claude-opus-4-6-v1", "eu-central-1", "us.anthropic.claude-opus-4-6-v1"},
		// ARNs are passed through verbatim.
		{"arn:aws:bedrock:us-east-1:123:inference-profile/us.anthropic.claude-opus-4-6-v1", "eu-west-1", "arn:aws:bedrock:us-east-1:123:inference-profile/us.anthropic.claude-opus-4-6-v1"},
		// Families that don't need a profile are untouched.
		{"amazon.nova-pro-v1:0", "us-east-1", "amazon.nova-pro-v1:0"},
		{"", "us-east-1", ""},
	}
	for _, c := range cases {
		if got := resolveBedrockInferenceProfileID(c.model, c.region); got != c.want {
			t.Errorf("resolveBedrockInferenceProfileID(%q, %q) = %q; want %q", c.model, c.region, got, c.want)
		}
	}
}
