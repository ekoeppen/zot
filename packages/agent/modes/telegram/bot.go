package telegram

import (
	"strings"
	"unicode/utf8"
)

// chunkMessage splits s into chunks no larger than limit runes, on line
// boundaries when possible.
func chunkMessage(s string, limit int) []string {
	if limit <= 0 || utf8.RuneCountInString(s) <= limit {
		return []string{s}
	}
	var out []string
	lines := strings.Split(s, "\n")
	var cur strings.Builder
	curRunes := 0
	for _, l := range lines {
		lineRunes := utf8.RuneCountInString(l)
		sepRunes := 0
		if curRunes > 0 {
			sepRunes = 1
		}
		if curRunes+sepRunes+lineRunes > limit && curRunes > 0 {
			out = append(out, cur.String())
			cur.Reset()
			curRunes = 0
			sepRunes = 0
		}
		if lineRunes > limit {
			// Line itself too long; hard-split on rune boundaries.
			for lineRunes > limit {
				i := byteIndexAfterRunes(l, limit)
				out = append(out, l[:i])
				l = l[i:]
				lineRunes = utf8.RuneCountInString(l)
			}
		}
		if curRunes > 0 {
			cur.WriteString("\n")
			curRunes++
		}
		cur.WriteString(l)
		curRunes += utf8.RuneCountInString(l)
	}
	if curRunes > 0 {
		out = append(out, cur.String())
	}
	return out
}

func byteIndexAfterRunes(s string, n int) int {
	if n <= 0 {
		return 0
	}
	count := 0
	for i := range s {
		if count == n {
			return i
		}
		count++
	}
	return len(s)
}

// isImageMIME returns true for MIME types the model can probably ingest
// as a vision input.
func isImageMIME(m string) bool {
	switch strings.ToLower(m) {
	case "image/png", "image/jpeg", "image/jpg", "image/gif", "image/webp":
		return true
	}
	return false
}

// guessImageMIME infers a mime type from a filename suffix. Falls back
// to image/png because telegram photos are always re-encoded to jpeg
// but getFile's file_path may omit the extension.
func guessImageMIME(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	}
	return "image/jpeg"
}
