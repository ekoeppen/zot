package modes

import "testing"

func newGateTestInteractive() *Interactive {
	return &Interactive{
		toolGate: map[string]int{},
		dirty:    make(chan struct{}, 1),
	}
}

// While paced text is still draining, a tool that arrives mid-stream
// must stay gated until the streaming buffer reaches the position
// captured when the tool started.
func TestToolGateHoldsToolUntilTextDrains(t *testing.T) {
	i := newGateTestInteractive()

	// Simulate a turn that streamed 10 runes already with 20 more
	// queued in the pacer when the tool call arrives.
	i.streamOn = true
	i.streaming.WriteString("0123456789") // 10 painted
	i.streamPending = []rune("01234567890123456789")

	i.gateToolLocked("t1")
	if got := i.toolGate["t1"]; got != 30 {
		t.Fatalf("gate = %d, want 30 (10 painted + 20 pending)", got)
	}

	if i.toolGateOpenLocked("t1") {
		t.Fatal("tool should be gated while only 10/30 runes painted")
	}

	// Drain halfway: still gated.
	i.streaming.WriteString("0123456789") // 20 painted
	if i.toolGateOpenLocked("t1") {
		t.Fatal("tool should still be gated at 20/30")
	}

	// Reach the gate: now visible.
	i.streaming.WriteString("0123456789") // 30 painted
	if !i.toolGateOpenLocked("t1") {
		t.Fatal("tool should be visible once streaming reaches the gate")
	}
}

// A tool call on a turn with no active text stream shows immediately.
func TestToolGateOpenWhenNoStream(t *testing.T) {
	i := newGateTestInteractive()
	i.streamOn = false

	i.gateToolLocked("t1")
	if got := i.toolGate["t1"]; got != 0 {
		t.Fatalf("gate = %d, want 0 for non-streaming turn", got)
	}
	if !i.toolGateOpenLocked("t1") {
		t.Fatal("tool should be visible immediately when no text is streaming")
	}
}

// First registration wins: a later EvToolCall must not move an
// existing gate (e.g. push it forward as more text arrives).
func TestToolGateFirstRegistrationWins(t *testing.T) {
	i := newGateTestInteractive()
	i.streamOn = true
	i.streaming.WriteString("01234") // 5
	i.streamPending = []rune("01234")

	i.gateToolLocked("t1") // gate 10
	first := i.toolGate["t1"]

	// More text queues up, then the same tool is re-registered.
	i.streamPending = append(i.streamPending, []rune("567890")...)
	i.gateToolLocked("t1")

	if i.toolGate["t1"] != first {
		t.Fatalf("gate moved from %d to %d; first registration must win", first, i.toolGate["t1"])
	}
}

// Once streaming finalizes, the buffer resets to length 0; gates that
// were already satisfied must not re-hide their tools.
func TestOpenAllToolGatesSurvivesStreamReset(t *testing.T) {
	i := newGateTestInteractive()
	i.streamOn = true
	i.streaming.WriteString("0123456789")
	i.gateToolLocked("t1") // gate 10, satisfied

	if !i.toolGateOpenLocked("t1") {
		t.Fatal("precondition: tool should be open before reset")
	}

	// Finalize the stream (mirrors resetStreamingStateLocked / pacer
	// flush): buffer resets to 0 but gates are opened first.
	i.openAllToolGatesLocked()
	i.streaming.Reset()
	i.streamOn = false

	if !i.toolGateOpenLocked("t1") {
		t.Fatal("tool re-hidden after stream reset; gate should have been opened")
	}
}
