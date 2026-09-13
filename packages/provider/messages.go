package provider

import (
	"encoding/json"
	"time"
)

// UnmarshalMessage parses a JSON-encoded Message, properly instantiating
// the concrete Content block types (TextBlock, ImageBlock, ToolCallBlock,
// ToolResultBlock, ReasoningBlock).
func UnmarshalMessage(rawMessage []byte) (Message, error) {
	var row struct {
		Role           Role              `json:"role"`
		Content        json.RawMessage   `json:"content"`
		Time           time.Time         `json:"time"`
		Meta           map[string]string `json:"meta,omitempty"`
		AddedToolNames []string          `json:"added_tool_names,omitempty"`
	}
	if err := json.Unmarshal(rawMessage, &row); err != nil {
		return Message{}, err
	}
	msg := Message{
		Role:           row.Role,
		Time:           row.Time,
		Meta:           row.Meta,
		AddedToolNames: row.AddedToolNames,
	}

	if len(row.Content) == 0 {
		return msg, nil
	}

	// Handle case where content is a single string primitive: "content": "hello"
	if row.Content[0] == '"' {
		var text string
		if err := json.Unmarshal(row.Content, &text); err == nil {
			msg.Content = []Content{TextBlock{Text: text}}
			return msg, nil
		}
	}

	var contentRaw []json.RawMessage
	if err := json.Unmarshal(row.Content, &contentRaw); err != nil {
		return msg, nil
	}

	for _, raw := range contentRaw {
		var head struct {
			Text             string `json:"text"`
			MimeType         string `json:"mime_type"`
			Data             []byte `json:"data"`
			ID               string `json:"id"`
			Name             string `json:"name"`
			CallID           string `json:"call_id"`
			ReasoningID      string `json:"reasoning_id"`
			Summary          string `json:"summary"`
			Encrypted        string `json:"encrypted_content"`
			ThoughtSignature string `json:"thought_signature"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			continue
		}
		// Discriminate by presence of fields.
		switch {
		case head.ReasoningID != "" || head.Summary != "" || head.Encrypted != "":
			msg.Content = append(msg.Content, ReasoningBlock{
				ID:        head.ReasoningID,
				Summary:   head.Summary,
				Encrypted: head.Encrypted,
			})
		case head.Name != "" && head.ID != "":
			var tc struct {
				ID               string          `json:"id"`
				Name             string          `json:"name"`
				Arguments        json.RawMessage `json:"arguments"`
				ThoughtSignature string          `json:"thought_signature"`
				Server           bool            `json:"server"`
			}
			_ = json.Unmarshal(raw, &tc)
			msg.Content = append(msg.Content, ToolCallBlock{
				ID:               tc.ID,
				Name:             tc.Name,
				Arguments:        tc.Arguments,
				ThoughtSignature: tc.ThoughtSignature,
				Server:           tc.Server,
			})
		case head.CallID != "":
			var tr struct {
				CallID  string            `json:"call_id"`
				Content []json.RawMessage `json:"content"`
				IsError bool              `json:"is_error"`
			}
			_ = json.Unmarshal(raw, &tr)
			block := ToolResultBlock{CallID: tr.CallID, IsError: tr.IsError}
			for _, c := range tr.Content {
				var inner struct {
					Format   string `json:"format"`
					Text     string `json:"text"`
					MimeType string `json:"mime_type"`
					Data     []byte `json:"data"`
				}
				_ = json.Unmarshal(c, &inner)
				if inner.MimeType != "" {
					block.Content = append(block.Content, ImageBlock{MimeType: inner.MimeType, Data: inner.Data})
				} else {
					block.Content = append(block.Content, TextBlock{Text: inner.Text, Format: inner.Format})
				}
			}
			msg.Content = append(msg.Content, block)
		case head.MimeType != "":
			msg.Content = append(msg.Content, ImageBlock{
				MimeType:         head.MimeType,
				Data:             head.Data,
				ThoughtSignature: head.ThoughtSignature,
			})
		default:
			msg.Content = append(msg.Content, TextBlock{
				Text:             head.Text,
				ThoughtSignature: head.ThoughtSignature,
			})
		}
	}
	return msg, nil
}

// UnmarshalMessages parses a JSON-encoded array of Messages.
func UnmarshalMessages(raw []byte) ([]Message, error) {
	var rawList []json.RawMessage
	if err := json.Unmarshal(raw, &rawList); err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(rawList))
	for _, rawMsg := range rawList {
		msg, err := UnmarshalMessage(rawMsg)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, nil
}
