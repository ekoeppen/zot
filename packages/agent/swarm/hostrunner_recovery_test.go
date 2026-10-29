package swarm

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patriceckhart/zot/packages/continuous"
	"github.com/patriceckhart/zot/packages/provider"
)

type reconnectHostClient struct {
	started  chan struct{}
	release  chan struct{}
	requests atomic.Int32
}

func (*reconnectHostClient) Name() string { return "reconnect-test" }
func (c *reconnectHostClient) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	if c.requests.Add(1) == 1 {
		close(c.started)
		select {
		case <-c.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return (echoHostClient{}).Stream(ctx, req)
}

func TestHostRunnerReconnectRecoversMissedSettlementWithoutReexecuting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	model := &reconnectHostClient{started: make(chan struct{}), release: make(chan struct{})}
	_, rt, baseDial := newHostFixtureWithClient(t, model)
	clients := make(chan *continuous.Client, 4)
	reconnect := make(chan struct{})
	var dials atomic.Int32
	dial := func(ctx context.Context) (*continuous.Client, error) {
		if dials.Add(1) > 1 {
			select {
			case <-reconnect:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		client, err := baseDial(ctx)
		if err == nil {
			clients <- client
		}
		return client, err
	}
	f := New(Config{Root: t.TempDir(), RepoRoot: t.TempDir(), NewRunner: NewHostRunnerFactory(dial, "recovery")})
	a, err := f.Spawn(ctx, "finish while disconnected")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopQueued(t, f, a) })
	completed := make(chan string, 4)
	a.SetOnTurnEnd(func(_ int, msg string) { completed <- msg })
	var first *continuous.Client
	select {
	case first = <-clients:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-model.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	first.Close()
	close(model.release)
	waitUntil(t, "offline task settlement", func() bool {
		snap, err := rt.ConversationSnapshot(ctx, a.Snapshot().ConversationID, 100)
		return err == nil && snap.Run != nil && snap.Run.Phase == "done"
	})
	select {
	case <-completed:
		t.Fatal("reported before reconnect")
	default:
	}
	close(reconnect)
	select {
	case msg := <-completed:
		if msg != "" {
			t.Fatal(msg)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if got := model.requests.Load(); got != 1 {
		t.Fatalf("task executed %d times", got)
	}
	if got := strings.Count(strings.Join(a.Transcript(), "\n"), "echo: finish while disconnected"); got != 1 {
		t.Fatalf("response mirrored %d times", got)
	}
	results := make(chan QueuedTurn, 1)
	if _, err := f.QueueTurn(a.ID, "follow up after reconnect", func(turn QueuedTurn) bool { results <- turn; return true }); err != nil {
		t.Fatal(err)
	}
	select {
	case turn := <-results:
		if turn.State != "completed" || turn.Response != "echo: follow up after reconnect" {
			t.Fatalf("follow-up result: %+v", turn)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestHostRunnerQueuedFollowupAfterResume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _, dial := newHostFixture(t)
	f := New(Config{Root: t.TempDir(), RepoRoot: t.TempDir(), NewRunner: NewHostRunnerFactory(dial, "resume-queue")})
	a, err := f.Spawn(ctx, "initial")
	if err != nil {
		t.Fatal(err)
	}
	initial := make(chan struct{}, 2)
	a.SetOnTurnEnd(func(int, string) { initial <- struct{}{} })
	select {
	case <-initial:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stopQueued(t, f, a)
	results := make(chan QueuedTurn, 1)
	if _, err := f.QueueTurn(a.ID, "queued while stopped", func(turn QueuedTurn) bool { results <- turn; return true }); err != nil {
		t.Fatal(err)
	}
	resumed, err := f.Resume(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Per-call callbacks are intentionally not persisted. Resume uses the global
	// observer, just as a fresh process does.
	t.Cleanup(func() { stopQueued(t, f, resumed) })
	waitUntil(t, "resumed follow-up", func() bool {
		turns := resumed.QueuedTurns()
		return len(turns) == 1 && turns[0].State == "completed"
	})
	turns := resumed.QueuedTurns()
	if turns[0].Response != "echo: queued while stopped" {
		t.Fatalf("resumed result: %+v", turns)
	}
}
