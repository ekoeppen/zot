package provider

import "testing"

func TestReasoningEffortMappings(t *testing.T) {
	cases := []struct {
		level      string
		openai     string
		codex      string
		budget     int
		normalized string
	}{
		{"off", "", "", 0, ""},
		{"minimum", "low", "low", 1024, "minimum"},
		{"minimal", "low", "low", 1024, "minimum"},
		{"low", "low", "low", 2048, "low"},
		{"medium", "medium", "medium", 8192, "medium"},
		{"high", "high", "high", 16384, "high"},
		{"maximum", "high", "xhigh", 32768, "maximum"},
		{"max", "high", "xhigh", 32768, "maximum"},
	}
	for _, tc := range cases {
		if got := NormalizeReasoning(tc.level); got != tc.normalized {
			t.Errorf("NormalizeReasoning(%q)=%q want %q", tc.level, got, tc.normalized)
		}
		if got := OpenAIReasoningEffort(tc.level); got != tc.openai {
			t.Errorf("OpenAIReasoningEffort(%q)=%q want %q", tc.level, got, tc.openai)
		}
		if got := OpenAICodexReasoningEffort(tc.level); got != tc.codex {
			t.Errorf("OpenAICodexReasoningEffort(%q)=%q want %q", tc.level, got, tc.codex)
		}
		if got := ReasoningBudget(tc.level); got != tc.budget {
			t.Errorf("ReasoningBudget(%q)=%d want %d", tc.level, got, tc.budget)
		}
	}
}
