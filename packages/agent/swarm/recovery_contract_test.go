package swarm

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequestIDSurvivesPromptValidation(t *testing.T) {
	data, err := json.Marshal(Prompt{RequestID: "request-123", Text: "text\nwith lines", Images: nil})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := DecodePrompt("user-prompt " + string(data))
	if err != nil || prompt.RequestID != "request-123" || prompt.Text != "text\nwith lines" {
		t.Fatalf("prompt: %+v %v", prompt, err)
	}
	for _, id := range []string{strings.Repeat("x", 129), "private\nrequest"} {
		data, _ := json.Marshal(Prompt{Text: "task", RequestID: id})
		if _, err := DecodePrompt("user-prompt " + string(data)); err == nil || strings.Contains(err.Error(), "private") {
			t.Fatal("invalid request ID accepted or exposed")
		}
	}
}

func TestQueueRejectsJSONExpansionBeforeAcceptance(t *testing.T) {
	mgr := New(Config{Root: t.TempDir(), RepoRoot: t.TempDir(), NewRunner: func(*Agent) Runner { return &queueRunner{input: make(chan string, 1)} }})
	a, err := mgr.Spawn(context.Background(), "initial")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopQueued(t, mgr, a) })
	// This fits in the raw text limit but JSON escapes every byte sixfold.
	if _, err := mgr.QueueTurn(a.ID, strings.Repeat("\x00", MaxPromptWireBytes/5), nil); err == nil {
		t.Fatal("accepted oversized wire frame")
	}
	if len(a.QueuedTurns()) != 0 {
		t.Fatal("rejected work was retained")
	}
}

func TestQueueNoticeRetriesWhenSessionChangesDuringCallback(t *testing.T) {
	r := &queueRunner{input: make(chan string, 1)}
	called := make(chan struct{}, 1)
	delivered := make(chan QueuedTurn, 1)
	release := make(chan struct{})
	var attempts atomic.Int32
	mgr := New(Config{Root: t.TempDir(), RepoRoot: t.TempDir(), NewRunner: func(*Agent) Runner { return r }})
	mgr.SetActiveSession("original")
	a, err := mgr.Spawn(context.Background(), "initial")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopQueued(t, mgr, a) })
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	if _, err := mgr.QueueTurn(a.ID, "follow-up", func(turn QueuedTurn) bool {
		if attempts.Add(1) == 1 {
			called <- struct{}{}
			<-release
			return false
		}
		delivered <- turn
		return true
	}); err != nil {
		t.Fatal(err)
	}
	notifyPromptTurnEnd(a, NewEvent("agent_ready", nil))
	prompt := receiveQueued(t, r, "follow-up")
	finishQueue(a, prompt, "answer")
	select {
	case <-called:
	case <-time.After(3 * time.Second):
		t.Fatal("no callback")
	}
	mgr.SetActiveSession("original")
	close(release)
	select {
	case turn := <-delivered:
		if turn.Response != "answer" {
			t.Fatalf("wrong result: %+v", turn)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("session wake was lost")
	}
}

type deadlineFailureConn struct {
	net.Conn
	deadline time.Time
	closed   bool
}

func (c *deadlineFailureConn) SetWriteDeadline(deadline time.Time) error {
	c.deadline = deadline
	return nil
}
func (*deadlineFailureConn) Write([]byte) (int, error) { return 0, os.ErrDeadlineExceeded }
func (c *deadlineFailureConn) Close() error            { c.closed = true; return nil }

func TestInboxDropsConnectionAfterBoundedWriteFailure(t *testing.T) {
	conn := &deadlineFailureConn{}
	inbox := &Inbox{conn: conn}
	before := time.Now()
	if err := inbox.SendInput("cancel"); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("write: %v", err)
	}
	if !conn.deadline.After(before) || conn.deadline.After(before.Add(3*time.Second)) {
		t.Fatalf("missing bounded deadline: %s", conn.deadline)
	}
	if !conn.closed || inbox.conn != nil {
		t.Fatal("failed socket retained")
	}
}
