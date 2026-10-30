package tui

import (
	"strings"
	"testing"
)

func TestStatusBarCompactionCount(t *testing.T) {
	for _, cols := range []int{200, 40, 20} {
		for _, count := range []int{0, 1, 12} {
			lines := StatusBar(StatusBarParams{Theme: Dark, Provider: "test", Model: "model", Compactions: count, Cols: cols})
			text := strings.Join(lines, "\n")
			if strings.Contains(text, "compactions:") != (count > 0) {
				t.Fatalf("count %d, width %d: %q", count, cols, text)
			}
			if count == 12 && !strings.Contains(text, "compactions: 12") {
				t.Fatalf("missing count: %q", text)
			}
			for _, line := range lines {
				if visibleWidth(line) > cols {
					t.Fatalf("line exceeds width %d: %q", cols, line)
				}
			}
		}
	}
}
