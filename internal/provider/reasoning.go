package provider

import "strings"

// NormalizeReasoning canonicalizes zot's user-facing thinking levels.
// Empty string means reasoning/thinking is disabled.
func NormalizeReasoning(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "off", "none", "no", "false", "disabled":
		return ""
	case "min", "minimal", "minimum":
		return "minimum"
	case "low":
		return "low"
	case "med", "medium":
		return "medium"
	case "hi", "high":
		return "high"
	case "max", "maximum":
		return "maximum"
	default:
		return strings.ToLower(strings.TrimSpace(level))
	}
}

// ReasoningBudget returns zot's approximate token budget for thinking-capable
// providers that accept explicit budgets.
func ReasoningBudget(level string) int {
	switch NormalizeReasoning(level) {
	case "minimum":
		return 1024
	case "low":
		return 2048
	case "medium":
		return 8192
	case "high":
		return 16384
	case "maximum":
		return 32768
	default:
		return 0
	}
}

// OpenAIReasoningEffort maps zot's six-level setting onto the effort enum
// accepted by OpenAI-compatible chat-completions endpoints.
func OpenAIReasoningEffort(level string) string {
	switch NormalizeReasoning(level) {
	case "minimum", "low":
		// Many OpenAI-compatible endpoints only accept low/medium/high.
		// Use low for zot's minimum instead of the newer minimal enum.
		return "low"
	case "medium":
		return "medium"
	case "high", "maximum":
		return "high"
	default:
		return ""
	}
}

// OpenAICodexReasoningEffort maps zot levels onto the ChatGPT/Codex
// Responses backend enum. That backend rejects "minimal" and uses
// "xhigh" for the highest tier on recent GPT-5.x models.
func OpenAICodexReasoningEffort(level string) string {
	switch NormalizeReasoning(level) {
	case "minimum", "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "maximum":
		return "xhigh"
	default:
		return ""
	}
}
