package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVertexConfigParsesAuthorizedUser(t *testing.T) {
	// Write a fake ADC user-OAuth file and point GOOGLE_APPLICATION_CREDENTIALS at it.
	tmp := t.TempDir()
	path := filepath.Join(tmp, "adc.json")
	body := `{
	  "type": "authorized_user",
	  "client_id": "fake.apps.googleusercontent.com",
	  "client_secret": "fake-secret",
	  "refresh_token": "1//fake-refresh"
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-proj")
	t.Setenv("GOOGLE_CLOUD_API_KEY", "") // ensure SA / user path is chosen
	cfg, err := loadVertexConfig()
	if err != nil {
		t.Fatalf("loadVertexConfig: %v", err)
	}
	if cfg.userClientID != "fake.apps.googleusercontent.com" {
		t.Errorf("client_id = %q", cfg.userClientID)
	}
	if cfg.userRefreshToken != "1//fake-refresh" {
		t.Errorf("refresh_token = %q", cfg.userRefreshToken)
	}
	if cfg.userTokenURI != "https://oauth2.googleapis.com/token" {
		t.Errorf("token_uri default not applied: %q", cfg.userTokenURI)
	}
	if cfg.cacheKey() != "user:fake.apps.googleusercontent.com" {
		t.Errorf("cacheKey = %q", cfg.cacheKey())
	}
}

type vertexRoundTripFunc func(*http.Request) (*http.Response, error)

func (f vertexRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestVertexTransportEndpoint(t *testing.T) {
	// vertexTransport only ever carries Gemini-shaped (generateContent)
	// requests now; Claude requests are rewritten by
	// vertexAnthropicTransport instead (see TestVertexAnthropicTransport).
	tests := []struct {
		name     string
		location string
		model    string
		wantHost string
	}{
		{name: "global gemini", location: "global", model: "gemini-test", wantHost: "aiplatform.googleapis.com"},
		{name: "regional gemini", location: "us-central1", model: "gemini-test", wantHost: "us-central1-aiplatform.googleapis.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *http.Request
			transport := &vertexTransport{
				cfg: &vertexConfig{
					project:  "test-project",
					location: tt.location,
					apiKey:   "test-key",
				},
				inner: vertexRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					got = req
					return &http.Response{StatusCode: http.StatusOK}, nil
				}),
			}
			const verb = ":streamGenerateContent"
			req, err := http.NewRequest(http.MethodPost, "https://placeholder.example/v1beta/models/"+tt.model+verb+"?alt=sse", nil)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := transport.RoundTrip(req); err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			if got.URL.Host != tt.wantHost {
				t.Errorf("host = %q, want %q", got.URL.Host, tt.wantHost)
			}
			wantPath := "/v1/projects/test-project/locations/" + tt.location + "/publishers/google/models/" + tt.model + verb
			if got.URL.Path != wantPath {
				t.Errorf("path = %q, want %q", got.URL.Path, wantPath)
			}
		})
	}
}

func TestVertexAnthropicTransport(t *testing.T) {
	tests := []struct {
		name     string
		location string
		wantHost string
	}{
		{name: "global", location: "global", wantHost: "aiplatform.googleapis.com"},
		{name: "regional", location: "us-central1", wantHost: "us-central1-aiplatform.googleapis.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *http.Request
			var gotBody []byte
			transport := &vertexAnthropicTransport{
				cfg: &vertexConfig{
					project:  "test-project",
					location: tt.location,
					apiKey:   "test-key",
				},
				inner: vertexRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					got = req
					gotBody, _ = io.ReadAll(req.Body)
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
				}),
			}
			// Mirrors what anthropicClient.Stream actually sends: a body
			// with a top-level "model" field and no anthropic_version
			// (that travels as a header on the public API, which we also
			// set here to verify it gets stripped).
			reqBody := `{"model":"claude-haiku-5-5","max_tokens":1024,"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"stream":true}`
			req, err := http.NewRequest(http.MethodPost, "https://placeholder.example/v1/messages", strings.NewReader(reqBody))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("x-api-key", "anthropic-key-should-be-dropped")
			req.Header.Set("anthropic-version", "2023-06-01")
			req.Header.Set("accept", "text/event-stream")

			if _, err := transport.RoundTrip(req); err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			if got.URL.Host != tt.wantHost {
				t.Errorf("host = %q, want %q", got.URL.Host, tt.wantHost)
			}
			wantPath := "/v1/projects/test-project/locations/" + tt.location + "/publishers/anthropic/models/claude-haiku-5-5:streamRawPredict"
			if got.URL.Path != wantPath {
				t.Errorf("path = %q, want %q", got.URL.Path, wantPath)
			}
			if got.Header.Get("x-api-key") != "" {
				t.Errorf("x-api-key header should be dropped, got %q", got.Header.Get("x-api-key"))
			}
			if got.Header.Get("x-goog-api-key") != "test-key" {
				t.Errorf("x-goog-api-key = %q, want test-key", got.Header.Get("x-goog-api-key"))
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(gotBody, &payload); err != nil {
				t.Fatalf("body is not valid JSON: %v (%s)", err, gotBody)
			}
			if _, ok := payload["model"]; ok {
				t.Errorf("rewritten body must not contain model field (it's in the URL): %s", gotBody)
			}
			var version string
			if raw, ok := payload["anthropic_version"]; ok {
				_ = json.Unmarshal(raw, &version)
			}
			if version != vertexAnthropicVersion {
				t.Errorf("anthropic_version = %q, want %q", version, vertexAnthropicVersion)
			}
			if _, ok := payload["messages"]; !ok {
				t.Errorf("rewritten body dropped messages: %s", gotBody)
			}
		})
	}
}

func TestVertexModelRouterDispatchesByModelID(t *testing.T) {
	var geminiCalled, anthropicCalled bool
	router := &vertexModelRouter{
		gemini:    fakeStreamClient(func(req Request) { geminiCalled = true }),
		anthropic: fakeStreamClient(func(req Request) { anthropicCalled = true }),
	}

	if _, err := router.Stream(context.Background(), Request{Model: "gemini-2.5-pro"}); err != nil {
		t.Fatalf("Stream (gemini): %v", err)
	}
	if !geminiCalled || anthropicCalled {
		t.Errorf("gemini model routed wrong: gemini=%v anthropic=%v", geminiCalled, anthropicCalled)
	}

	geminiCalled, anthropicCalled = false, false
	if _, err := router.Stream(context.Background(), Request{Model: "claude-haiku-5-5"}); err != nil {
		t.Fatalf("Stream (claude): %v", err)
	}
	if geminiCalled || !anthropicCalled {
		t.Errorf("claude model routed wrong: gemini=%v anthropic=%v", geminiCalled, anthropicCalled)
	}
}

type fakeStreamClient func(req Request)

func (f fakeStreamClient) Name() string { return "fake" }
func (f fakeStreamClient) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	f(req)
	ch := make(chan Event)
	close(ch)
	return ch, nil
}

func TestVertexConfigRejectsBadType(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "adc.json")
	body := `{"type": "something_else"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-proj")
	t.Setenv("GOOGLE_CLOUD_API_KEY", "")
	_, err := loadVertexConfig()
	if err == nil {
		t.Fatal("expected error for unknown credential type")
	}
}
