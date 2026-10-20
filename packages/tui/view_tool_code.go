package tui

import (
	"encoding/json"
	"strings"

	"github.com/patriceckhart/zot/packages/provider"
)

// toolCodeInput is only used for code-running tools other than codemode, whose
// legacy raw-source and streaming formats have their own renderer.
type toolCodeInput struct {
	Code string
	Lang string
}

func inferToolCodeLang(tool string) string {
	name := strings.ToLower(tool)
	switch {
	case name == "eval_lg" || strings.Contains(name, "clojure") || strings.Contains(name, "let_go") || strings.Contains(name, "let-go"):
		return "clojure"
	case strings.Contains(name, "javascript") || strings.HasSuffix(name, "_js") || strings.HasPrefix(name, "js_") || name == "js":
		return "javascript"
	case strings.Contains(name, "python") || strings.HasSuffix(name, "_py") || strings.HasPrefix(name, "py_") || name == "py":
		return "python"
	case strings.Contains(name, "sql"):
		return "sql"
	case strings.Contains(name, "bash") || name == "sh" || strings.HasSuffix(name, "_sh"):
		return "bash"
	case strings.Contains(name, "ruby") || strings.HasSuffix(name, "_rb") || name == "rb":
		return "ruby"
	case strings.Contains(name, "lua"):
		return "lua"
	default:
		return ""
	}
}

func codeFromToolArgs(tool string, raw json.RawMessage) (toolCodeInput, bool) {
	if tool == "codemode" {
		return toolCodeInput{}, false
	}
	var args map[string]json.RawMessage
	if json.Unmarshal(raw, &args) != nil {
		return toolCodeInput{}, false
	}
	lang := inferToolCodeLang(tool)
	for _, field := range []string{"lang", "language"} {
		var explicit string
		if json.Unmarshal(args[field], &explicit) == nil && strings.TrimSpace(explicit) != "" {
			lang = strings.ToLower(strings.TrimSpace(explicit))
			break
		}
	}
	if lang == "" {
		return toolCodeInput{}, false
	}
	for _, field := range []string{"code", "query", "expression", "script"} {
		var code string
		if json.Unmarshal(args[field], &code) == nil && strings.TrimSpace(code) != "" {
			return toolCodeInput{code, lang}, true
		}
	}
	return toolCodeInput{}, false
}

func hasToolCode(tool, raw string) bool {
	_, ok := codeFromToolArgs(tool, json.RawMessage(raw))
	return ok
}

func partialToolCode(tool, raw string) (toolCodeInput, bool) {
	if tool == "codemode" {
		return toolCodeInput{}, false
	}
	lang := inferToolCodeLang(tool)
	for _, field := range []string{"lang", "language"} {
		if explicit, ok, _ := ExtractPartialStringField(raw, field); ok && strings.TrimSpace(explicit) != "" {
			lang = strings.ToLower(strings.TrimSpace(explicit))
			break
		}
	}
	if lang == "" {
		return toolCodeInput{}, false
	}
	for _, field := range []string{"code", "query", "expression", "script"} {
		if code, ok, _ := ExtractPartialStringField(raw, field); ok && strings.TrimSpace(code) != "" {
			return toolCodeInput{code, lang}, true
		}
	}
	return toolCodeInput{}, false
}

// Keep the upstream codemode result path unchanged. Non-codemode scripts are
// appended after output so the result remains visible in shortened previews.
func (v *View) renderToolResultWithCode(blocks []provider.Content, width, color int, path string, startLine int, source string, code toolCodeInput) []string {
	if source != "" || code.Code == "" {
		return v.renderToolResultWithSource(blocks, width, color, path, startLine, source)
	}
	body := v.renderToolResultContent(blocks, width, color, path, startLine)
	if v.CollapseToolCall && !v.ExpandAll {
		return body
	}
	if len(body) > 0 {
		body = append(body, "")
	}
	body = append(body, v.renderToolCodeBlock(code.Code, code.Lang, width)...)
	for _, block := range blocks {
		if _, ok := block.(provider.ImageBlock); ok {
			return body // image footprint rows must not be collapsed
		}
	}
	return v.collapseToolBody(body, false)
}

func (v *View) renderToolCodeBlock(code, lang string, width int) []string {
	bodyWidth := toolBoxBodyRenderWidth(width)
	if v.FlatTools || v.CompactMode {
		bodyWidth = flatToolBodyRenderWidth(width)
	}
	lines := strings.Split(code, "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}
	for i, line := range lines {
		lines[i] = expandTabs(line)
	}
	highlighted := v.Theme.HighlightCode(strings.Join(lines, "\n"), lang)
	var out []string
	for _, line := range highlighted {
		for _, wrapped := range wrapANSILine(line, max(1, bodyWidth-4)) {
			out = append(out, "    "+wrapped)
		}
	}
	return out
}
