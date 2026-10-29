package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patriceckhart/zot/packages/agent/swarm"
)

func TestSwarmInboxQueuesManualAndAutomatedInputAndCancelsActiveTurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	lines := make(chan string)
	events := make(chan swarm.Event, 20)
	started := make(chan string, 4)
	done := make(chan error, 1)
	var active atomic.Int32
	go func() {
		done <- serveSwarmPrompts(ctx, swarm.Prompt{Text: "initial"}, lines, func(typ string, data map[string]any) {
			events <- swarm.NewEvent(typ, data)
		}, func(ctx context.Context, p swarm.Prompt) (string, error) {
			if active.Add(1) != 1 {
				t.Error("concurrent prompts")
			}
			defer active.Add(-1)
			started <- p.Text
			if p.Text == "initial" {
				<-ctx.Done()
				return "", ctx.Err()
			}
			return "answer: " + p.Text, nil
		})
	}()
	takeStarted := func(want string) {
		t.Helper()
		select {
		case got := <-started:
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	send := func(msg string) {
		t.Helper()
		select {
		case lines <- msg:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	takeStarted("initial")
	send("user manual")
	data, _ := json.Marshal(swarm.Prompt{Text: "automated", RequestID: "request-123"})
	send("user-prompt " + string(data))
	send("cancel")
	takeStarted("manual")
	takeStarted("automated")
	var ends []swarm.Event
	for len(ends) < 3 {
		select {
		case ev := <-events:
			if ev.Type == "error" {
				t.Fatalf("input dropped: %+v", ev)
			}
			if ev.Type == "turn_end" {
				ends = append(ends, ev)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if !strings.Contains(ends[0].Data["error"].(string), "canceled") {
		t.Fatalf("initial not cancelled: %+v", ends[0])
	}
	if ends[2].Data["request_id"] != "request-123" || ends[2].Data["response"] != "answer: automated" {
		t.Fatalf("wrong correlation: %+v", ends[2])
	}
	send("shutdown")
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestSwarmInboxShutdownWaitsForActivePrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	lines := make(chan string)
	started, stopped := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- serveSwarmPrompts(ctx, swarm.Prompt{Text: "blocking"}, lines, func(string, map[string]any) {}, func(ctx context.Context, _ swarm.Prompt) (string, error) {
			close(started)
			<-ctx.Done()
			close(stopped)
			return "", ctx.Err()
		})
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case lines <- "shutdown":
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-stopped:
	default:
		t.Fatal("daemon returned before prompt cleanup")
	}
}
