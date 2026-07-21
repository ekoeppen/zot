package modes

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/patriceckhart/zot/packages/core"
	"github.com/patriceckhart/zot/packages/tui"
)

func TestConfirmDialogAllowsToolExpansion(t *testing.T) {
	resp := make(chan core.ConfirmDecision, 1)
	dialog := newConfirmDialog()
	dialog.Enqueue(&confirmRequest{
		toolName: "edit",
		preview:  "large edit",
		resp:     resp,
	})
	args, err := json.Marshal(map[string]any{
		"path": "sample.ts",
		"edits": []map[string]string{{
			"oldText": "old",
			"newText": strings.Repeat("const value = 1\n", 20),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	view := &tui.View{
		ToolCalls: []tui.ToolCallView{{
			ID:         "call-1",
			Name:       "edit",
			RawJSONBuf: string(args),
			LivePath:   "sample.ts",
		}},
	}
	i := &Interactive{
		view:          view,
		confirmDialog: dialog,
		dirty:         make(chan struct{}, 1),
	}
	if rendered := strings.Join(view.Build(80), "\n"); !strings.Contains(rendered, "ctrl+o to expand") {
		t.Fatalf("test preview was not initially collapsed:\n%s", rendered)
	}

	i.handleKey(context.Background(), tui.Key{Kind: tui.KeyCtrlO})

	if !i.view.ExpandAll {
		t.Fatal("ctrl+o did not expand tool previews while confirmation was active")
	}
	if rendered := strings.Join(view.Build(80), "\n"); strings.Contains(rendered, "ctrl+o to expand") {
		t.Fatalf("live edit preview remained collapsed after ctrl+o:\n%s", rendered)
	}
	if !dialog.Active() {
		t.Fatal("ctrl+o closed the confirmation dialog")
	}
	select {
	case decision := <-resp:
		t.Fatalf("ctrl+o unexpectedly answered confirmation: %+v", decision)
	default:
	}

	i.handleKey(context.Background(), tui.Key{Kind: tui.KeyCtrlO})
	if i.view.ExpandAll {
		t.Fatal("second ctrl+o did not collapse tool previews during confirmation")
	}
	if rendered := strings.Join(view.Build(80), "\n"); !strings.Contains(rendered, "ctrl+o to expand") {
		t.Fatalf("live edit preview remained expanded after second ctrl+o:\n%s", rendered)
	}
	if !dialog.Active() {
		t.Fatal("second ctrl+o closed the confirmation dialog")
	}
}
