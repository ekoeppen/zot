package extproto

import (
	"encoding/json"
	"testing"
)

func TestBeforeModelRequestPayloads(t *testing.T) {
	req := EventInterceptFromHost{
		Type:       "event_intercept",
		ID:         "req-1",
		Event:      "before_model_request",
		SessionID:  "session-123",
		AgentRunID: "run-456",
		CWD:        "/work",
		Provider:   "anthropic",
		Model:      "claude-opus-4-7",
		Messages:   json.RawMessage(`[{"role":"user","content":[{"type":"text","text":"hi"}]}]`),
	}

	encoded, err := Encode(req)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	var decoded EventInterceptFromHost
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Event != "before_model_request" || string(decoded.Messages) != string(req.Messages) {
		t.Fatalf("decoded = %+v, want %+v", decoded, req)
	}

	resp := EventInterceptResponseFromExt{
		Type:     "event_intercept_response",
		ID:       "req-1",
		Messages: json.RawMessage(`[{"role":"user","content":[{"type":"text","text":"transformed"}]}]`),
	}
	respEncoded, err := Encode(resp)
	if err != nil {
		t.Fatalf("Encode response failed: %v", err)
	}

	var respDecoded EventInterceptResponseFromExt
	if err := json.Unmarshal(respEncoded, &respDecoded); err != nil {
		t.Fatalf("Unmarshal response failed: %v", err)
	}
	if string(respDecoded.Messages) != string(resp.Messages) {
		t.Fatalf("respDecoded = %+v, want %+v", respDecoded, resp)
	}
}
