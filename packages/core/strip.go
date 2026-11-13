package core

import (
	"strings"

	"github.com/patriceckhart/zot/packages/provider"
)

const toolImageMirrorPrefix = "Tool output included the following image content:"

// StripToolCalls removes all tool call blocks, tool result blocks, synthetic
// tool-image mirror messages, and any messages left without visible non-tool
// content (such as tool-only assistant turns or role=tool messages). When
// tool content was removed, it also drops any leading non-user messages so the
// resulting transcript starts with a user turn, and preserves activated
// deferred tool names and tool_state metadata on the remaining messages.
func StripToolCalls(msgs []provider.Message) (stripped []provider.Message, removedCalls int, removedResults int) {
	if len(msgs) == 0 {
		return nil, 0, 0
	}
	out := make([]provider.Message, 0, len(msgs))
	removedMirror := false
	for _, m := range msgs {
		if isToolImageMirrorMessage(m) {
			removedMirror = true
			continue
		}
		var filtered []provider.Content
		hasVisible := false
		msgRemovedTool := false
		for _, c := range m.Content {
			switch v := c.(type) {
			case provider.ToolCallBlock:
				removedCalls++
				msgRemovedTool = true
			case provider.ToolResultBlock:
				removedResults++
				msgRemovedTool = true
			case provider.TextBlock:
				filtered = append(filtered, v)
				if strings.TrimSpace(v.Text) != "" {
					hasVisible = true
				}
			case provider.ImageBlock:
				filtered = append(filtered, v)
				hasVisible = true
			default:
				// Retain non-tool metadata blocks (e.g. ReasoningBlock) only when
				// visible non-tool content remains in the same message.
				filtered = append(filtered, v)
			}
		}
		if m.Role == provider.RoleTool {
			continue
		}
		if msgRemovedTool && !hasVisible {
			continue
		}
		if len(filtered) == 0 {
			continue
		}
		cp := m
		cp.Content = filtered
		if _, hasNested := cp.Meta[provider.NestedToolCallsMetaKey]; hasNested {
			meta := make(map[string]string, len(cp.Meta))
			for k, val := range cp.Meta {
				if k != provider.NestedToolCallsMetaKey {
					meta[k] = val
				}
			}
			if len(meta) == 0 {
				meta = nil
			}
			cp.Meta = meta
		}
		out = append(out, cp)
	}
	if removedCalls == 0 && removedResults == 0 && !removedMirror {
		return append([]provider.Message(nil), msgs...), 0, 0
	}
	for len(out) > 0 && out[0].Role != provider.RoleUser {
		out = out[1:]
	}
	if len(out) == 0 {
		return nil, removedCalls, removedResults
	}

	var activatedTools []string
	for _, m := range msgs {
		for _, name := range m.AddedToolNames {
			if !containsString(activatedTools, name) {
				activatedTools = append(activatedTools, name)
			}
		}
	}
	if len(activatedTools) > 0 {
		first := out[0]
		merged := append([]string(nil), first.AddedToolNames...)
		for _, name := range activatedTools {
			if !containsString(merged, name) {
				merged = append(merged, name)
			}
		}
		first.AddedToolNames = merged
		out[0] = first
	}

	if stateMeta := toolStateMetadata(readToolState(msgs)); len(stateMeta) > 0 {
		last := out[len(out)-1]
		meta := make(map[string]string, len(last.Meta)+len(stateMeta))
		for k, v := range last.Meta {
			meta[k] = v
		}
		for k, v := range stateMeta {
			meta[k] = v
		}
		last.Meta = meta
		out[len(out)-1] = last
	}

	return out, removedCalls, removedResults
}

// StripToolCalls removes all tool calls and tool responses from the agent's
// transcript in-place, updating revision and persisting a compaction
// checkpoint when anything changed.
func (a *Agent) StripToolCalls() (removedCalls int, removedResults int) {
	a.mu.Lock()
	stripped, calls, results := StripToolCalls(a.messages)
	if calls == 0 && results == 0 && len(stripped) == len(a.messages) {
		a.mu.Unlock()
		return 0, 0
	}
	a.messages = stripped
	a.rev++
	onCompacted := a.OnTranscriptCompacted
	persisted := append([]provider.Message(nil), stripped...)
	a.mu.Unlock()

	if onCompacted != nil {
		onCompacted(persisted)
	}
	return calls, results
}

func isToolImageMirrorMessage(m provider.Message) bool {
	if m.Role != provider.RoleUser || len(m.Content) == 0 {
		return false
	}
	tb, ok := m.Content[0].(provider.TextBlock)
	if !ok {
		return false
	}
	return strings.TrimSpace(tb.Text) == toolImageMirrorPrefix
}
