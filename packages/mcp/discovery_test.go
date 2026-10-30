package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// discoveryHTTP blocks initialize until released or cancelled. Tests control
// progress explicitly, without depending on network or connection timing.
func discoveryHTTP(t *testing.T, name string) (string, <-chan struct{}, func()) {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req rpcRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil || len(req.ID) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if req.Method == "initialize" {
			enteredOnce.Do(func() { close(entered) })
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		result := fixtureResult(req)
		if req.Method == "tools/list" {
			result = map[string]any{"tools": []any{map[string]any{"name": name, "inputSchema": map[string]any{"type": "object"}}}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(unblock)
	return srv.URL, entered, unblock
}

func awaitDiscovery(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !ready() {
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("discovery did not reach the expected state")
		}
	}
}

func awaitDiscoveryDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("discovery operation did not finish")
	}
}

func TestBackgroundDiscoveryPublishesServersIndependently(t *testing.T) {
	fast, fastEntered, releaseFast := discoveryHTTP(t, "fast")
	slow, slowEntered, releaseSlow := discoveryHTTP(t, "slow")
	home := t.TempDir()
	writeConfig(t, filepath.Join(home, "mcp.json"), fmt.Sprintf(`{"mcpServers":{"fast":{"url":%q},"slow":{"url":%q},"off":{"enabled":false}}}`, fast, slow))
	m := New(home, t.TempDir())
	t.Cleanup(m.Close)
	if errs := m.StartDiscovery(context.Background()); len(errs) != 0 {
		t.Fatal(errs)
	}
	awaitDiscoveryDone(t, fastEntered)
	awaitDiscoveryDone(t, slowEntered)
	snap := m.Snapshot()
	if len(snap.Servers) != 3 || snap.Servers[0].State != StateConnecting || snap.Servers[1].State != StateConnecting || snap.Servers[2].State != StateDisabled {
		t.Fatalf("initial snapshot=%+v", snap)
	}
	releaseFast()
	awaitDiscovery(t, func() bool { return registry(m)["mcp__fast__fast"] != nil })
	if snap = m.Snapshot(); snap.Servers[0].Name != "slow" || snap.Servers[0].State != StateConnecting {
		t.Fatalf("fast server was held up by slow server: %+v", snap)
	}

	// A cancelled prompt must not cancel the shared startup discovery or
	// consume its pending catalog update.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if m.RefreshStale(ctx) {
		t.Fatal("cancelled prompt refreshed the registry")
	}
	releaseSlow()
	if !m.RefreshStale(context.Background()) {
		t.Fatal("first prompt did not receive discovered tools")
	}
	if registry(m)["mcp__slow__slow"] == nil {
		t.Fatal("slow server's tools were not published")
	}
	if m.RefreshStale(context.Background()) {
		t.Fatal("completed discovery caused repeated registry refreshes")
	}
}

func TestBackgroundDiscoveryCloseAndReloadCancelPending(t *testing.T) {
	for _, action := range []string{"close", "reload", "cd"} {
		t.Run(action, func(t *testing.T) {
			url, entered, release := discoveryHTTP(t, "obsolete")
			home, cwd := t.TempDir(), t.TempDir()
			path := filepath.Join(cwd, ".zot", "mcp.json")
			writeConfig(t, path, fmt.Sprintf(`{"mcpServers":{"old":{"url":%q,"connectTimeout":60}}}`, url))
			m := New(home, cwd)
			t.Cleanup(m.Close)
			if errs := m.StartDiscovery(context.Background()); len(errs) != 0 {
				t.Fatal(errs)
			}
			awaitDiscoveryDone(t, entered)
			if action == "reload" {
				writeConfig(t, path, `{"mcpServers":{"new":{"enabled":false}}}`)
			}
			nextCWD := t.TempDir()
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				switch action {
				case "close":
					m.Close()
				case "reload":
					if errs := m.Reload(context.Background()); len(errs) != 0 {
						t.Error(errs)
					}
				case "cd":
					m.SetCWD(nextCWD)
					if errs := m.Reload(context.Background()); len(errs) != 0 {
						t.Error(errs)
					}
				}
			}()
			awaitDiscoveryDone(t, finished)
			release()
			if len(m.Tools()) != 0 {
				t.Fatal("obsolete discovery restored tools")
			}
			snap := m.Snapshot()
			if action == "reload" {
				if len(snap.Servers) != 1 || snap.Servers[0].Name != "new" || snap.Servers[0].State != StateDisabled {
					t.Fatalf("replacement snapshot=%+v", snap)
				}
			} else if len(snap.Servers) != 0 {
				t.Fatalf("obsolete servers remained: %+v", snap)
			}
		})
	}
}

func TestBackgroundDiscoveryImmediateFailures(t *testing.T) {
	home := t.TempDir()
	configs := map[string]ServerConfig{}
	for idx := 0; idx < 64; idx++ {
		configs[fmt.Sprintf("s%02d", idx)] = ServerConfig{Command: filepath.Join(home, "missing-server")}
	}
	data, err := json.Marshal(map[string]any{"mcpServers": configs})
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, filepath.Join(home, "mcp.json"), string(data))
	m := New(home, t.TempDir())
	t.Cleanup(m.Close)
	if errs := m.StartDiscovery(context.Background()); len(errs) != 0 {
		t.Fatal(errs)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !m.RefreshStale(ctx) {
		t.Fatal("failed discovery did not complete")
	}
	snap := m.Snapshot()
	if len(snap.Servers) != len(configs) {
		t.Fatalf("only %d servers published", len(snap.Servers))
	}
	for _, s := range snap.Servers {
		if s.State != StateFailed || s.Problem != "command not found" {
			t.Fatalf("failed server=%+v", s)
		}
	}
}

func TestBackgroundDiscoveryTimeoutIsReported(t *testing.T) {
	url, entered, _ := discoveryHTTP(t, "never-ready")
	home := t.TempDir()
	writeConfig(t, filepath.Join(home, "mcp.json"), fmt.Sprintf(`{"mcpServers":{"slow":{"url":%q,"connectTimeout":1}}}`, url))
	m := New(home, t.TempDir())
	t.Cleanup(m.Close)
	if errs := m.StartDiscovery(context.Background()); len(errs) != 0 {
		t.Fatal(errs)
	}
	awaitDiscoveryDone(t, entered)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !m.RefreshStale(ctx) {
		t.Fatal("discovery did not finish within its connection deadline")
	}
	snap := m.Snapshot()
	if len(snap.Servers) != 1 || snap.Servers[0].State != StateFailed || snap.Servers[0].Problem != "timed out while connecting" || len(m.Tools()) != 0 {
		t.Fatalf("timeout snapshot=%+v", snap)
	}
}

func TestBackgroundDiscoveryReconnectRetiresOnlyOneAttempt(t *testing.T) {
	old, oldEntered, _ := discoveryHTTP(t, "obsolete")
	other, otherEntered, _ := discoveryHTTP(t, "other")
	replacement, _, release := discoveryHTTP(t, "replacement")
	release()
	home := t.TempDir()
	path := filepath.Join(home, "mcp.json")
	config := func(url string) string {
		return fmt.Sprintf(`{"mcpServers":{"s":{"url":%q},"other":{"url":%q}}}`, url, other)
	}
	writeConfig(t, path, config(old))
	m := New(home, t.TempDir())
	t.Cleanup(m.Close)
	if errs := m.StartDiscovery(context.Background()); len(errs) != 0 {
		t.Fatal(errs)
	}
	awaitDiscoveryDone(t, oldEntered)
	awaitDiscoveryDone(t, otherEntered)
	writeConfig(t, path, config(replacement))
	if err := m.Reconnect(context.Background(), "s"); err != nil {
		t.Fatal(err)
	}
	if registry(m)["mcp__s__replacement"] == nil || registry(m)["mcp__s__obsolete"] != nil {
		t.Fatal("replacement did not retire old tools")
	}
	snap := m.Snapshot()
	if snap.Servers[0].Name != "other" || snap.Servers[0].State != StateConnecting {
		t.Fatalf("reconnect disrupted other pending server: %+v", snap)
	}
}
