package telegram

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestChunkMessageUsesRuneLimit(t *testing.T) {
	s := strings.Repeat("界", 4001)
	chunks := chunkMessage(s, 4000)
	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2", len(chunks))
	}
	if got := utf8.RuneCountInString(chunks[0]); got != 4000 {
		t.Fatalf("first chunk runes = %d, want 4000", got)
	}
	if got := utf8.RuneCountInString(chunks[1]); got != 1 {
		t.Fatalf("second chunk runes = %d, want 1", got)
	}
}

func TestChunkMessageDoesNotSplitMultiByteRune(t *testing.T) {
	chunks := chunkMessage("🙂🙂🙂", 2)
	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2", len(chunks))
	}
	if chunks[0] != "🙂🙂" || chunks[1] != "🙂" {
		t.Fatalf("chunks = %#v", chunks)
	}
}
