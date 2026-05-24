package tui

import "testing"

func TestEditorMoveVerticalStyledPromptKeepsColumn(t *testing.T) {
	e := NewEditor("\x1b[38;5;111m▌ \x1b[0m")
	e.SetValue("alpha beta gamma delta epsilon zeta eta theta")
	_, _, _ = e.Render(18)

	// Put cursor on a lower wrapped row, then move up. With the styled
	// prompt included in wrap geometry this used to jump near the start of
	// the top row instead of the cell directly above.
	e.CursorC = 28
	_, _, beforeCol := e.Render(18)
	if !e.MoveVertical(-1) {
		t.Fatal("expected vertical move")
	}
	_, _, afterCol := e.Render(18)
	if afterCol != beforeCol {
		t.Fatalf("column changed: before=%d after=%d cursor=%d", beforeCol, afterCol, e.CursorC)
	}
}
