package tui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/patriceckhart/zot/packages/provider"
)

func timestampTestMessage(at time.Time) provider.Message {
	return provider.Message{Role: provider.RoleAssistant, Time: at, Content: []provider.Content{provider.TextBlock{Text: "same text"}}}
}

func TestChatTimestampDateDisplayAndMargin(t *testing.T) {
	start := time.Date(2026, 9, 22, 23, 58, 0, 0, time.Local)
	v := View{Theme: Dark, ChatTimestamps: true, Messages: []provider.Message{
		timestampTestMessage(start), timestampTestMessage(start.Add(time.Minute)), timestampTestMessage(start.Add(2 * time.Minute)),
	}}
	for _, mode := range []string{"", "day_start", "every_message", "invalid"} {
		v.ChatTimestampDate = mode
		want := []string{"2026-09-22 23:58", "23:59", "2026-09-23 00:00"}
		if mode == "every_message" {
			want[1] = "2026-09-22 23:59"
		}
		if got := v.messageTimestamps(); !reflect.DeepEqual(got, want) {
			t.Fatalf("mode %q: got %v, want %v", mode, got, want)
		}
		for _, width := range []int{20, 40, 80} {
			row := stripANSI(v.timestampRow(want[0], width))
			box := stripANSI(toolBoxBottom(Dark, width))
			if len(strings.TrimRight(row, " ")) != visibleWidth(strings.TrimRight(box, " ")) {
				t.Fatalf("width %d: timestamp does not align with box right edge", width)
			}
		}
	}
}

func TestChatTimestampIntervals(t *testing.T) {
	start := time.Date(2026, 9, 22, 12, 0, 0, 0, time.Local)
	five := 5
	v := View{ChatTimestamps: true, ChatTimestampIntervalMinutes: &five}
	for _, delta := range []time.Duration{0, 2 * time.Minute, 5*time.Minute - time.Second, 5 * time.Minute, 6 * time.Minute, 10 * time.Minute} {
		m := timestampTestMessage(start.Add(delta))
		m.Role = provider.RoleUser
		v.Messages = append(v.Messages, m)
	}
	want := []string{"2026-09-22 12:00", "", "", "12:05", "", "12:10"}
	if got := v.messageTimestamps(); !reflect.DeepEqual(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
	zero := 0
	v.ChatTimestampIntervalMinutes = nil
	for idx, label := range v.messageTimestamps() {
		if label == "" {
			t.Fatalf("every-message mode omitted message %d", idx)
		}
	}
	negative := -1
	if ChatTimestampInterval(nil) != 0 || ChatTimestampInterval(&negative) != 0 || ChatTimestampInterval(&zero) != 0 {
		t.Fatal("incorrect interval defaults")
	}
}

func TestChatTimestampFinalAnswerBypassesInterval(t *testing.T) {
	start := time.Date(2026, 9, 22, 12, 0, 0, 0, time.Local)
	five := 5
	prompt := timestampTestMessage(start)
	prompt.Role = provider.RoleUser
	intermediate := timestampTestMessage(start.Add(14 * time.Minute))
	intermediate.Content = append(intermediate.Content, provider.ToolCallBlock{ID: "call"})
	tool := provider.Message{Role: provider.RoleTool, Time: start.Add(14 * time.Minute), Content: []provider.Content{
		provider.ToolResultBlock{CallID: "call", Content: []provider.Content{provider.TextBlock{Text: "file written"}}},
	}}
	answer := timestampTestMessage(start.Add(15 * time.Minute))
	v := View{Theme: Dark, ChatTimestamps: true, ChatTimestampIntervalMinutes: &five, Messages: []provider.Message{
		prompt, intermediate, tool, answer,
	}}
	want := []string{"2026-09-22 12:00", "12:14", "", "12:15"}
	if got := v.messageTimestamps(); !reflect.DeepEqual(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
	if !strings.Contains(strings.Join(v.Build(80), "\n"), "12:15") {
		t.Fatal("final answer timestamp missing from rendered transcript")
	}

	// Intermediate assistant text with a tool call still obeys the interval.
	v.Messages[1].Time = start.Add(time.Minute)
	want = []string{"2026-09-22 12:00", "", "12:14", "12:15"}
	if got := v.messageTimestamps(); !reflect.DeepEqual(got, want) {
		t.Fatalf("intermediate labels = %v, want %v", got, want)
	}
	// A tool timestamp immediately before the answer must not suppress it.
	v.Messages = []provider.Message{prompt, tool, answer}
	want = []string{"2026-09-22 12:00", "12:14", "12:15"}
	if got := v.messageTimestamps(); !reflect.DeepEqual(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
	// Completing a short answer also qualifies. Later prompts do not remove
	// its timestamp, and interval selection resumes from the answer's time.
	v.Messages[2].Time = start.Add(14*time.Minute + time.Second)
	next := timestampTestMessage(start.Add(16 * time.Minute))
	next.Role = provider.RoleUser
	v.Messages = append(v.Messages, next)
	want = []string{"2026-09-22 12:00", "12:14", "12:14", ""}
	if got := v.messageTimestamps(); !reflect.DeepEqual(got, want) {
		t.Fatalf("labels after next prompt = %v, want %v", got, want)
	}
}

func TestChatTimestampMissingHiddenAndDayChange(t *testing.T) {
	start := time.Date(2026, 9, 22, 23, 59, 0, 0, time.Local)
	hidden := timestampTestMessage(start.Add(time.Minute))
	hidden.Meta = map[string]string{"compaction": "true"}
	toolOnly := provider.Message{Role: provider.RoleAssistant, Time: start.Add(time.Minute), Content: []provider.Content{provider.ToolCallBlock{ID: "call"}}}
	v := View{ChatTimestamps: true, Messages: []provider.Message{
		timestampTestMessage(time.Time{}), timestampTestMessage(start), hidden, toolOnly,
		timestampTestMessage(start.Add(time.Minute)),
	}}
	want := []string{"", "2026-09-22 23:59", "", "", "2026-09-23 00:00"}
	if got := v.messageTimestamps(); !reflect.DeepEqual(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
}

func TestChatTimestampsPreserveRowsAndCaches(t *testing.T) {
	start := time.Date(2026, 9, 22, 12, 0, 0, 0, time.Local)
	for _, compact := range []bool{false, true} {
		v := View{Theme: Dark, CompactMode: compact, Messages: []provider.Message{
			timestampTestMessage(start), timestampTestMessage(start.Add(5 * time.Minute)),
		}}
		v.Messages[0].Role = provider.RoleUser
		plain, anchors := v.BuildWithAnchors(40)
		v.ChatTimestamps = true
		stamped, stampedAnchors := v.BuildWithAnchors(40)
		if len(plain) != len(stamped) || !reflect.DeepEqual(anchors, stampedAnchors) {
			t.Fatal("timestamps changed transcript height or anchors")
		}
		for idx, line := range stamped {
			if line != plain[idx] && (plain[idx] != "" || visibleWidth(line) != 40) {
				t.Fatalf("timestamp overwrote content or was not right aligned: %q", line)
			}
		}
		if !strings.Contains(strings.Join(stamped, "\n"), v.Theme.FG256(v.Theme.Muted, "12:05")) {
			t.Fatal("missing muted timestamp")
		}
		v.Messages[1].Time = start.Add(6 * time.Minute)
		if !strings.Contains(strings.Join(v.Build(40), "\n"), "12:06") {
			t.Fatal("content cache reused the wrong timestamp")
		}
		v.ChatTimestamps = false
		if got := v.Build(40); !reflect.DeepEqual(got, plain) {
			t.Fatal("disabling timestamps did not restore original rows")
		}
	}
}

func TestChatTimestampTailAndNarrowWidths(t *testing.T) {
	start := time.Date(2026, 9, 22, 12, 0, 0, 0, time.Local)
	v := View{Theme: Dark, ChatTimestamps: true, Messages: []provider.Message{
		timestampTestMessage(start), timestampTestMessage(start.Add(time.Minute)), timestampTestMessage(start.Add(5 * time.Minute)),
	}}
	all, anchors := v.BuildWithAnchors(40)
	v.TailLimit = 2
	if got := v.Build(40); !reflect.DeepEqual(got, all[anchors[1].Row:]) {
		t.Fatal("tail rendering changed timestamp selection")
	}
	for _, width := range []int{0, 1, 4, 5, 15, 16, 40} {
		for _, label := range []string{"12:00", "2026-09-22 12:00"} {
			line := v.timestampRow(label, width)
			if visibleWidth(line) > width || (width-2*toolBoxOuterMargin < len(label) && line != "") {
				t.Fatalf("width %d: label overflow: %q", width, line)
			}
		}
	}
}

func TestChatTimestampsToolLayoutsAndClockRegression(t *testing.T) {
	start := time.Date(2026, 9, 22, 12, 0, 0, 0, time.Local)
	zero := 0
	for _, layout := range []string{"box", "flat", "compact"} {
		v := View{Theme: Dark, FlatTools: layout == "flat", CompactMode: layout == "compact", CollapseToolCall: true,
			ChatTimestampIntervalMinutes: &zero, Messages: []provider.Message{
				timestampTestMessage(start),
				{Role: provider.RoleTool, Time: start.Add(-time.Minute), Content: []provider.Content{
					provider.ToolResultBlock{CallID: "call", Content: []provider.Content{provider.TextBlock{Text: "tool output"}}},
				}},
			},
		}
		plain := v.Build(80)
		v.ChatTimestamps = true
		stamped := v.Build(80)
		if len(plain) != len(stamped) || !strings.Contains(strings.Join(stamped, "\n"), "11:59") {
			t.Fatalf("%s: tool timestamp missing or added rows", layout)
		}
		five := 5
		v.ChatTimestampIntervalMinutes = &five
		if v.messageTimestamps()[1] != "" {
			t.Fatal("clock regression bypassed positive minimum interval")
		}
	}
}

func TestChatTimestampsWaitForCompletedMessages(t *testing.T) {
	v := View{Theme: Dark, StreamingActive: true, Streaming: "streaming text"}
	before := v.Build(80)
	liveBefore := v.BuildLive(80)
	v.ChatTimestamps = true
	if !reflect.DeepEqual(before, v.Build(80)) || !reflect.DeepEqual(liveBefore, v.BuildLive(80)) {
		t.Fatal("timestamps modified in-progress streaming output")
	}
	v.StreamingActive = false
	v.Streaming = ""
	v.Messages = []provider.Message{timestampTestMessage(time.Date(2026, 9, 22, 12, 0, 0, 0, time.Local))}
	if !strings.Contains(strings.Join(v.Build(80), "\n"), "2026-09-22 12:00") {
		t.Fatal("completed message missing timestamp")
	}
}
