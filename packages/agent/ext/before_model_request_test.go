package ext

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/patriceckhart/zot/packages/agent/extproto"
	"github.com/patriceckhart/zot/packages/provider"
)

func TestBeforeModelRequestSDK(t *testing.T) {
	for _, mode := range []string{"transform", "omit"} {
		t.Run(mode, func(t *testing.T) {
			h := newHarness("model-req-ext")
			h.ext.InterceptBeforeModelRequest(func(ev BeforeModelRequestEvent) []provider.Message {
				if len(ev.Messages) != 1 || ev.CWD != "/work" || ev.Provider != "p" || ev.Model != "m" || ev.SessionID != "s" {
					t.Errorf("wrong event metadata: %+v", ev)
				}
				if mode == "omit" {
					return nil
				}
				return []provider.Message{
					{Role: provider.RoleUser, Content: []provider.Content{provider.TextBlock{Text: "transformed"}}},
				}
			})

			done := make(chan struct{})
			go func() { defer close(done); h.ext.Run() }()
			t.Cleanup(func() { h.hostW.Close(); <-done; h.ext.out.(io.Closer).Close() })

			h.drainUntil(t, "hello")
			h.sendToExt(t, extproto.HelloAckFromHost{Type: "hello_ack", ProtocolVersion: extproto.ProtocolVersion})
			sub := h.drainUntil(t, "subscribe")
			var subscription extproto.SubscribeFromExt
			if err := json.Unmarshal(sub.raw, &subscription); err != nil {
				t.Fatal(err)
			}
			if len(subscription.Intercept) != 1 || subscription.Intercept[0] != "before_model_request" {
				t.Fatalf("subscription = %+v", subscription)
			}
			h.drainUntil(t, "ready")

			rawMsgs := json.RawMessage(`[{"role":"user","content":[{"type":"text","text":"original"}]}]`)
			h.sendToExt(t, extproto.EventInterceptFromHost{
				Type:      "event_intercept",
				Event:     "before_model_request",
				ID:        "1",
				Messages:  rawMsgs,
				CWD:       "/work",
				Provider:  "p",
				Model:     "m",
				SessionID: "s",
			})

			frame := h.drainUntil(t, "event_intercept_response")
			var result extproto.EventInterceptResponseFromExt
			if err := json.Unmarshal(frame.raw, &result); err != nil {
				t.Fatal(err)
			}
			if result.ID != "1" {
				t.Fatal("missing correlation ID")
			}

			if mode == "omit" {
				if len(result.Messages) != 0 {
					t.Fatalf("expected empty messages, got %s", result.Messages)
				}
			} else {
				msgs, err := provider.UnmarshalMessages(result.Messages)
				if err != nil {
					t.Fatalf("UnmarshalMessages failed: %v", err)
				}
				if len(msgs) != 1 {
					t.Fatalf("expected 1 message, got %d", len(msgs))
				}
				tb, ok := msgs[0].Content[0].(provider.TextBlock)
				if !ok || tb.Text != "transformed" {
					t.Fatalf("expected 'transformed', got %+v", msgs[0].Content)
				}
			}
		})
	}
}
