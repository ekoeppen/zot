package extensions

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/patriceckhart/zot/packages/agent/extproto"
	"github.com/patriceckhart/zot/packages/provider"
)

func addModelRequestExtension(t *testing.T, m *Manager, name string, response func(extproto.EventInterceptFromHost) string) *Extension {
	t.Helper()
	log, err := os.CreateTemp(t.TempDir(), "log")
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	ext := &Extension{
		Manifest:         Manifest{Name: name},
		logFile:          log,
		readyCh:          make(chan struct{}),
		pendingIntercept: map[string]chan extproto.EventInterceptResponseFromExt{},
		interceptSubs:    map[string]struct{}{},
		eventSubs:        map[string]struct{}{},
	}
	ext.stdin = startWriter{write: func(p []byte) (int, error) {
		var ev extproto.EventInterceptFromHost
		if err := json.Unmarshal(p, &ev); err != nil {
			return 0, err
		}
		_, err := io.WriteString(writer, response(ev)+"\n")
		return len(p), err
	}}
	m.mu.Lock()
	m.ext[name] = ext
	m.extOrder = append(m.extOrder, ext)
	m.mu.Unlock()
	done := make(chan struct{})
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	go func() { defer close(done); m.readLoop(ext, scanner) }()
	t.Cleanup(func() { writer.Close(); <-done; reader.Close(); log.Close() })
	if _, err := io.WriteString(writer, `{"type":"subscribe","intercept":["before_model_request"]}`+"\n"+`{"type":"ready"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	<-ext.readyCh
	return ext
}

func TestBeforeModelRequestResponses(t *testing.T) {
	for _, tc := range []struct {
		name     string
		raw      string
		wantText string
		warning  bool
	}{
		{"transformed", `[{"role":"user","content":[{"type":"text","text":"transformed"}]}]`, "transformed", false},
		{"omitted", "", "original", false},
		{"empty array", `[]`, "", false},
		{"invalid object", `{"invalid": true}`, "original", true},
		{"invalid number", `42`, "original", true},
		{"invalid element", `[123]`, "original", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hooks := &stubHooks{}
			m := New(t.TempDir(), "/effective", "test", "provider", "model", hooks)
			ext := addModelRequestExtension(t, m, "test", func(ev extproto.EventInterceptFromHost) string {
				if ev.Event != "before_model_request" || ev.CWD != "/effective" || ev.SessionID != "session" || ev.Provider != "provider" || ev.Model != "model" {
					t.Errorf("wrong event metadata: %+v", ev)
				}
				field := ""
				if tc.raw != "" {
					field = `,"messages":` + tc.raw
				}
				return `{"type":"event_intercept_response","id":"` + ev.ID + `"` + field + `}`
			})

			input := []provider.Message{
				{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: "original"}}},
			}
			got := m.InterceptBeforeModelRequest(context.Background(), extproto.EventInterceptFromHost{
				SessionID: "session",
				Model:     "model",
			}, input)

			if tc.wantText == "" {
				if len(got) != 0 {
					t.Fatalf("expected empty messages, got %d", len(got))
				}
			} else {
				if len(got) != 1 {
					t.Fatalf("got %d messages, want 1", len(got))
				}
				tb, ok := got[0].Content[0].(provider.TextBlock)
				if !ok || tb.Text != tc.wantText {
					t.Fatalf("got %q, want %q", tb.Text, tc.wantText)
				}
			}

			ext.mu.Lock()
			warnings := len(ext.diagnostics)
			ext.mu.Unlock()
			if (warnings > 0) != tc.warning {
				t.Fatalf("warnings = %d, want warning=%v", warnings, tc.warning)
			}
		})
	}
}

func TestBeforeModelRequestChainsInLoadOrder(t *testing.T) {
	m := New(t.TempDir(), "", "test", "p", "m", nil)
	for _, name := range []string{"ext-1", "ext-2"} {
		extName := name
		addModelRequestExtension(t, m, extName, func(ev extproto.EventInterceptFromHost) string {
			msgs, _ := provider.UnmarshalMessages(ev.Messages)
			tb, _ := msgs[0].Content[0].(provider.TextBlock)
			newMsg := []provider.Message{
				{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: tb.Text + "/" + extName}}},
			}
			raw, _ := json.Marshal(newMsg)
			return `{"type":"event_intercept_response","id":"` + ev.ID + `","messages":` + string(raw) + `}`
		})
	}

	input := []provider.Message{
		{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: "orig"}}},
	}
	got := m.InterceptBeforeModelRequest(context.Background(), extproto.EventInterceptFromHost{}, input)
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	tb, _ := got[0].Content[0].(provider.TextBlock)
	if tb.Text != "orig/ext-1/ext-2" {
		t.Fatalf("got text %q, want 'orig/ext-1/ext-2'", tb.Text)
	}
}
