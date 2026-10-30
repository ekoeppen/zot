package tui

import (
	"strings"
	"time"

	"github.com/patriceckhart/zot/packages/provider"
)

// ChatTimestampInterval returns the interval in minutes. Missing or negative
// values use zero minutes, showing every eligible message.
func ChatTimestampInterval(minutes *int) int {
	if minutes == nil || *minutes < 0 {
		return 0
	}
	return *minutes
}

// ChatTimestampDateMode normalizes the date display preference.
func ChatTimestampDateMode(mode string) string {
	if mode == "every_message" {
		return mode
	}
	return "day_start"
}

// messageTimestamps computes labels over the whole transcript, including the
// history outside TailLimit. Decoration stays outside the content render cache.
func (v *View) messageTimestamps() []string {
	if !v.ChatTimestamps {
		return nil
	}
	labels := make([]string, len(v.Messages))
	interval := ChatTimestampInterval(v.ChatTimestampIntervalMinutes)
	var last time.Time
	for idx, m := range v.Messages {
		if m.Time.IsZero() || !timestampMessageVisible(m) {
			continue
		}
		stamp := m.Time.Local()
		newDay := !last.IsZero() && stamp.Format("2006-01-02") != last.Format("2006-01-02")
		if interval > 0 && !last.IsZero() && !newDay && !timestampFinalAnswer(m) && stamp.Sub(last).Minutes() < float64(interval) {
			continue
		}
		format := "15:04"
		if ChatTimestampDateMode(v.ChatTimestampDate) == "every_message" || last.IsZero() || newDay {
			format = "2006-01-02 15:04"
		}
		labels[idx] = stamp.Format(format)
		last = stamp
	}
	return labels
}

// Completed assistant replies without tool calls end a response. Always label
// them, even when a recent tool result consumed the interval. Classifying by
// content keeps the label stable when later messages are appended or replayed.
func timestampFinalAnswer(m provider.Message) bool {
	if m.Role != provider.RoleAssistant {
		return false
	}
	for _, c := range m.Content {
		if _, ok := c.(provider.ToolCallBlock); ok {
			return false
		}
	}
	return true
}

func timestampMessageVisible(m provider.Message) bool {
	if m.Meta["compaction"] == "true" {
		return false
	}
	for _, c := range m.Content {
		switch b := c.(type) {
		case provider.TextBlock:
			if (m.Role == provider.RoleUser || m.Role == provider.RoleAssistant) && strings.TrimSpace(b.Text) != "" {
				return true
			}
		case provider.ImageBlock:
			if m.Role == provider.RoleUser {
				return true
			}
		case provider.ToolResultBlock:
			if m.Role == provider.RoleTool {
				return true
			}
		}
	}
	return false
}

// timestampRow occupies the already existing separator row, never content or
// extra vertical space. Narrow terminals omit labels that cannot fit in full.
func (v *View) timestampRow(label string, width int) string {
	if label == "" || width-2*toolBoxOuterMargin < len(label) {
		return ""
	}
	return strings.Repeat(" ", width-toolBoxOuterMargin-len(label)) + v.Theme.FG256(v.Theme.Muted, label) + strings.Repeat(" ", toolBoxOuterMargin)
}
