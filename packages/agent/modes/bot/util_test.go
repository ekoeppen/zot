package bot

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestChunkMessageShort(t *testing.T) {
	got := ChunkMessage("hello", 10)
	if len(got) != 1 || got[0] != "hello" {
		t.Fatalf("got %#v", got)
	}
}

func TestChunkMessageSplitsOnLines(t *testing.T) {
	in := strings.Repeat("aaaa\n", 5) + "bbbb"
	for _, c := range ChunkMessage(in, 10) {
		if utf8.RuneCountInString(c) > 10 {
			t.Fatalf("chunk too long: %q", c)
		}
	}
}

func TestChunkMessageHardSplitsLongLine(t *testing.T) {
	in := strings.Repeat("x", 25)
	got := ChunkMessage(in, 10)
	if len(got) != 3 {
		t.Fatalf("want 3 chunks, got %d: %#v", len(got), got)
	}
}

// Ported from telegram/bot_test.go to preserve rune-boundary coverage.
func TestChunkMessageUsesRuneLimit(t *testing.T) {
	s := strings.Repeat("界", 4001)
	chunks := ChunkMessage(s, 4000)
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
	chunks := ChunkMessage("🙂🙂🙂", 2)
	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2", len(chunks))
	}
	if chunks[0] != "🙂🙂" || chunks[1] != "🙂" {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestIsImageMIME(t *testing.T) {
	if !IsImageMIME("image/PNG") || IsImageMIME("application/pdf") {
		t.Fatal("mime gate wrong")
	}
}

func TestGuessImageMIME(t *testing.T) {
	if GuessImageMIME("a/b.webp") != "image/webp" {
		t.Fatal("webp")
	}
	if GuessImageMIME("photos/file") != "image/jpeg" {
		t.Fatal("fallback should be jpeg (telegram re-encodes photos)")
	}
}
