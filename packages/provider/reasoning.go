package provider

import "strings"

// NormalizeReasoning canonicalizes zot's user-facing thinking levels.
// Empty string means reasoning/thinking is disabled. "maximum" remains
// an alias for xhigh; "max" is the separate opt-in tier above it.
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
	case "xhigh", "maximum":
		return "xhigh"
	case "max":
		return "max"
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
	case "xhigh", "max":
		return 32768
	default:
		return 0
	}
}

// AnthropicAdaptiveEffort maps zot's user-facing thinking levels onto the
// effort enum used by adaptive-thinking models. These models reject explicit
// thinking budgets; reasoning depth is controlled by output_config.effort.
func AnthropicAdaptiveEffort(level string) string {
	switch NormalizeReasoning(level) {
	case "minimum", "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh":
		return "xhigh"
	case "max":
		return "max"
	default:
		return ""
	}
}

// OpenAIReasoningEffort maps zot's thinking setting onto the effort enum
// accepted by generic OpenAI-compatible chat-completions endpoints.
func OpenAIReasoningEffort(level string) string {
	switch NormalizeReasoning(level) {
	case "minimum", "low":
		// Many compatible endpoints only accept low/medium/high.
		return "low"
	case "medium":
		return "medium"
	case "high", "xhigh", "max":
		return "high"
	default:
		return ""
	}
}

// OpenAICompatAnthropicEffort maps zot's thinking setting when an adaptive
// Anthropic model is served over an OpenAI-compatible chat-completions wire.
// Adaptive models accept native xhigh and max effort values.
func OpenAICompatAnthropicEffort(level string) string {
	switch NormalizeReasoning(level) {
	case "minimum", "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh":
		return "xhigh"
	case "max":
		return "max"
	default:
		return ""
	}
}

// OpenAICodexReasoningEffort maps zot levels onto the Responses API effort
// enum. GPT-5.6 supports native max; other models clamp max to xhigh.
func OpenAICodexReasoningEffort(level, model string) string {
	switch NormalizeReasoning(level) {
	case "minimum", "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh":
		return "xhigh"
	case "max":
		if strings.HasPrefix(strings.ToLower(model), "gpt-5.6-") {
			return "max"
		}
		return "xhigh"
	default:
		return ""
	}
}
