package tui

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// TestEditor_AnsiPromptCursorAtEnd reproduces the live VS-Code drag
// and drop bug: when the editor's Prompt carries ANSI styling (the
// themed "▌ " glyph used by the interactive mode), the raw escape
// bytes leaked into wrapLine's per-rune width counter, and the
// cursor reported by Render() landed inside the wrapped row instead
// of at the end. Render() must return a column equal to the visible
// width of the last wrapped row when CursorC sits at end-of-buffer.
func TestEditor_AnsiPromptCursorAtEnd(t *testing.T) {
	// "\x1b[38;5;111m▌ \x1b[0m" is the exact prompt the interactive
	// mode constructs via cfg.Theme.AccentBar. We hard-code the
	// bytes here so the test doesn't depend on theme internals.
	ansiPrompt := "\x1b[38;5;111m▌ \x1b[0m"

	e := NewEditor(ansiPrompt)
	// Same scenario as the captured live session: a screencaptureui
	// temp path then " hello" typed afterwards.
	pasted := "'/var/folders/xq/hdh5qm6j66nbzd0sh3ljsxyc0000gn/T/TemporaryItems/NSIRD_screencaptureui_P22eIV/Screenshot 2026-05-13 at 11.09.28.png'"
	e.insert(pasted)
	for _, r := range " hello" {
		e.insert(string(r))
	}

	lines, row, col := e.Render(113)
	if len(lines) == 0 {
		t.Fatalf("Render returned no lines")
	}
	last := lines[len(lines)-1]
	wantRow := len(lines) - 1
	wantCol := runewidth.StringWidth(stripANSI(last))

	if row != wantRow || col != wantCol {
		t.Fatalf("cursor at row=%d col=%d, want row=%d col=%d\nrendered rows:\n%s",
			row, col, wantRow, wantCol, strings.Join(lines, "\n"))
	}
}
