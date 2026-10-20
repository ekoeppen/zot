package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/patriceckhart/zot/packages/provider"
)

func codeToolMessages(args json.RawMessage, result string) []provider.Message {
	return []provider.Message{
		{Role: provider.RoleAssistant, Content: []provider.Content{provider.ToolCallBlock{ID: "call", Name: "eval_lg", Arguments: args}}},
		{Role: provider.RoleTool, Content: []provider.Content{provider.ToolResultBlock{CallID: "call", Content: []provider.Content{provider.TextBlock{Text: result}}}}},
	}
}

func TestCodeToolStreamingAndCompleted(t *testing.T) {
	v := View{Theme: Dark}
	live := ToolCallView{Name: "eval_lg", Streaming: true, RawJSONBuf: `{"code":"(defn add [a b]\n  (+ a b))`}
	if got := stripANSI(strings.Join(v.RenderToolCall(live, 80), "\n")); !strings.Contains(got, "(+ a b))") {
		t.Fatalf("partial code missing: %s", got)
	}
	args := json.RawMessage(`{"code":"(+ 40 2)"}`)
	for _, layout := range []string{"box", "flat", "compact"} {
		v := View{Theme: Dark, FlatTools: layout == "flat", CompactMode: layout == "compact", Messages: codeToolMessages(args, "=> 42")}
		for _, rows := range [][]string{v.Build(80), v.RenderToolCall(ToolCallView{Name: "eval_lg", Done: true, RawJSONBuf: string(args), Result: "=> 42"}, 80)} {
			plain := stripANSI(strings.Join(rows, "\n"))
			if !strings.Contains(plain, "(+ 40 2)") || !strings.Contains(plain, "=> 42") || strings.Index(plain, "=> 42") > strings.LastIndex(plain, "(+ 40 2)") {
				t.Fatalf("result must precede code (%s): %s", layout, plain)
			}
			for _, row := range rows {
				if visibleWidth(row) > 80 {
					t.Fatalf("row exceeds terminal width: %q", stripANSI(row))
				}
			}
		}
	}
}

func TestCodeToolEmptyOutputAndCollapse(t *testing.T) {
	args, _ := json.Marshal(map[string]string{"code": strings.Repeat("(println 1)\n", ToolCollapseLines+10) + "SOURCE_END"})
	for _, collapsed := range []bool{false, true} {
		v := View{Theme: Dark, CollapseToolCall: collapsed, Messages: codeToolMessages(args, "RESULT_MARKER")}
		for _, expanded := range []bool{false, true} {
			v.ExpandAll = expanded
			for _, rows := range [][]string{v.Build(80), v.RenderToolCall(ToolCallView{Name: "eval_lg", Done: true, RawJSONBuf: string(args), Result: "RESULT_MARKER"}, 80)} {
				plain := stripANSI(strings.Join(rows, "\n"))
				if !strings.Contains(plain, "RESULT_MARKER") || strings.Contains(plain, "SOURCE_END") != expanded {
					t.Fatalf("result or expansion lost (collapsed=%v expanded=%v): %s", collapsed, expanded, plain)
				}
			}
		}
	}
	v := View{Theme: Dark, ExpandAll: true, Messages: codeToolMessages(json.RawMessage(`{"code":"(println 1)"}`), "")}
	if got := stripANSI(strings.Join(v.Build(80), "\n")); !strings.Contains(got, "(println 1)") {
		t.Fatalf("missing code for empty result: %s", got)
	}
	if got := stripANSI(strings.Join(v.RenderToolCall(ToolCallView{Name: "eval_lg", Done: true, RawJSONBuf: `{"code":"(println 1)"}`}, 80), "\n")); !strings.Contains(got, "(println 1)") {
		t.Fatalf("missing live code for empty result: %s", got)
	}
}

func TestCodeToolReplayCache(t *testing.T) {
	v := View{Theme: Dark, ExpandAll: true, Messages: codeToolMessages(json.RawMessage(`{"code":"OLD_SOURCE"}`), "result")}
	if got := stripANSI(strings.Join(v.Build(80), "\n")); !strings.Contains(got, "OLD_SOURCE") {
		t.Fatalf("missing replay code: %s", got)
	}
	v.Messages[0].Content[0] = provider.ToolCallBlock{ID: "call", Name: "eval_lg", Arguments: json.RawMessage(`{"code":"NEW_SOURCE"}`)}
	if got := stripANSI(strings.Join(v.Build(80), "\n")); strings.Contains(got, "OLD_SOURCE") || !strings.Contains(got, "NEW_SOURCE") {
		t.Fatalf("stale result cache: %s", got)
	}
}

func TestCodeToolExplicitLanguage(t *testing.T) {
	code, ok := codeFromToolArgs("custom", json.RawMessage(`{"query":"select 1", "language":"sql"}`))
	if !ok || code.Code != "select 1" || code.Lang != "sql" {
		t.Fatalf("unexpected code: %+v, %v", code, ok)
	}
	if _, ok := codeFromToolArgs("other", json.RawMessage(`{"code":"unrelated"}`)); ok {
		t.Fatal("unknown tool with no language should retain ordinary rendering")
	}
}
