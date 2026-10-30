package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/patriceckhart/zot/packages/agent/extensions"
	"github.com/patriceckhart/zot/packages/core"
)

func TestNonInteractiveMCPStillWaitsForDiscovery(t *testing.T) {
	t.Setenv("ZOT_HOME", t.TempDir())
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	url := mcpFixtureServerWithInitialize(t, func(ctx context.Context) {
		once.Do(func() { close(entered) })
		select {
		case <-release:
		case <-ctx.Done():
		}
	})
	if err := os.WriteFile(filepath.Join(ZotHome(), "mcp.json"), []byte(`{"mcpServers":{"docs":{"url":"`+url+`"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr := extensions.New(ZotHome(), t.TempDir(), "test", "", "", nonInteractiveExtHooks{})
	t.Cleanup(func() { mgr.Stop(time.Millisecond) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cwd := t.TempDir()
	done := make(chan []error, 1)
	go func() { done <- loadBuiltinMCP(ctx, mgr, Args{}, cwd) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("discovery did not start")
	}
	select {
	case <-done:
		t.Fatal("non-interactive loading returned before discovery")
	default:
	}
	close(release)
	select {
	case errs := <-done:
		if len(errs) != 0 {
			t.Fatal(errs)
		}
	case <-ctx.Done():
		t.Fatal("non-interactive loading did not finish")
	}
}

func TestInteractiveMCPDiscoveryWaitsAtPromptBoundary(t *testing.T) {
	t.Setenv("ZOT_HOME", t.TempDir())
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	url := mcpFixtureServerWithInitialize(t, func(ctx context.Context) {
		once.Do(func() { close(entered) })
		select {
		case <-release:
		case <-ctx.Done():
		}
	})
	if err := os.WriteFile(filepath.Join(ZotHome(), "mcp.json"), []byte(`{"mcpServers":{"docs":{"url":"`+url+`"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr := extensions.New(ZotHome(), t.TempDir(), "test", "", "", nonInteractiveExtHooks{})
	t.Cleanup(func() { mgr.Stop(time.Millisecond) })
	source := registerBuiltinMCP(mgr, Args{}, t.TempDir())
	if source == nil || !mgr.HasCommand("mcp") {
		t.Fatal("MCP commands unavailable before discovery")
	}
	select {
	case <-entered:
		t.Fatal("registration started discovery before UI initialization")
	default:
	}
	ag := core.NewAgent(nil, "test", "", core.NewRegistry(toolStub{"read"}))
	wireNonInteractiveAgentExtHooks(context.Background(), ag, mgr)
	if errs := source.StartDiscovery(context.Background()); len(errs) != 0 {
		t.Fatal(errs)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("discovery did not start")
	}

	// Use the real pre-turn hook. Cancelling a prompt releases the wait,
	// but does not cancel shared discovery or make a provider request.
	turnEntered := make(chan struct{})
	beforeTurn := ag.BeforeTurnContext
	ag.BeforeTurnContext = func(ctx context.Context, step int) (bool, string) {
		close(turnEntered)
		return beforeTurn(ctx, step)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- ag.Prompt(ctx, "hello", nil, nil) }()
	select {
	case <-turnEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("prompt did not reach discovery wait")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("prompt error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled prompt waited for network discovery")
	}
	if ag.ToolRegistry()["mcp__docs__lookup"] != nil {
		t.Fatal("uninitialized tool registered")
	}
	close(release)
	// This is the same refresh the next prompt performs before inference.
	mgr.RefreshBuiltinTools(context.Background())
	reg := ag.ToolRegistry()
	if reg["mcp__docs__lookup"] == nil || reg["read"] == nil {
		t.Fatalf("first ready prompt did not receive the merged registry: %v", reg)
	}
}
