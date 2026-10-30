package modes

import (
	"strings"
	"testing"

	"github.com/patriceckhart/zot/packages/mcp"
	"github.com/patriceckhart/zot/packages/tui"
)

func TestMCPManagerRefreshesConnectingServers(t *testing.T) {
	d := newMCPManagerDialog()
	snap := mcp.Snapshot{Loaded: true, Servers: []mcp.ServerInfo{
		{Name: "slow", State: mcp.StateConnecting},
		{Name: "ready", State: mcp.StateConnected, ToolCount: 1},
	}}
	d.Open(func() mcp.Snapshot { return snap }, false)
	if !d.NeedsTickRefresh() || !strings.Contains(renderPlain(d), "connecting") {
		t.Fatal("connecting state is not live in the manager")
	}
	d.HandleKey(tui.Key{Kind: tui.KeyDown})
	// Completion can reorder the snapshot. Preserve the selected server.
	snap.Servers = []mcp.ServerInfo{
		{Name: "ready", State: mcp.StateConnected, ToolCount: 1},
		{Name: "slow", State: mcp.StateConnected, ToolCount: 2},
	}
	out := renderPlain(d)
	if d.NeedsTickRefresh() || strings.Contains(out, "connecting") || !strings.Contains(out, "2/2 connected") || d.snap.Servers[d.cursor].Name != "ready" {
		t.Fatalf("completion did not update the manager: %s", out)
	}
	d.Close()
	if d.NeedsTickRefresh() {
		t.Fatal("closed dialog still requests ticks")
	}
}
