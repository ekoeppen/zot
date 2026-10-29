package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patriceckhart/zot/packages/continuous"
)

// Exercise the shipped CLI, real daemon sockets, provider streaming, session
// restoration, and separate supervisor processes against a loopback-only model.
func TestSwarmReleaseEndToEnd(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("requires a Go toolchain and Unix-domain sockets")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "zot")
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binary, "../../../cmd/zot")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build zot: %v\n%s", err, output)
	}
	var mu sync.Mutex
	counts := map[string]int{}
	started := make(chan string, 20)
	hostRelease := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected model endpoint: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var request struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		text := ""
		for _, msg := range request.Messages {
			if msg.Role != "user" {
				continue
			}
			if json.Unmarshal(msg.Content, &text) != nil {
				var blocks []struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(msg.Content, &blocks); err != nil {
					t.Error(err)
					return
				}
				text = ""
				for _, block := range blocks {
					text += block.Text
				}
			}
		}
		mu.Lock()
		counts[text]++
		mu.Unlock()
		started <- text
		w.Header().Set("Content-Type", "text/event-stream")
		if text == "blocked initial" {
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
			case <-ctx.Done():
			}
			return
		}
		if text == "finish while supervisor disconnected" {
			select {
			case <-hostRelease:
			case <-ctx.Done():
				return
			}
		}
		chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "echo: " + text}}}})
		fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", chunk)
	}))
	defer server.Close()

	t.Run("supervisor process restart preserves unsent work exactly once", func(t *testing.T) {
		root := t.TempDir()
		for _, phase := range []string{"accept", "resume"} {
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(ctx, self, "-test.run=^TestSwarmReleaseSupervisorProcess$")
			cmd.Env = append(os.Environ(), "ZOT_RELEASE_PHASE="+phase, "ZOT_RELEASE_BINARY="+binary, "ZOT_RELEASE_ROOT="+root, "ZOT_RELEASE_URL="+server.URL, "ZOT_HOME="+filepath.Join(root, "home"))
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s supervisor: %v\n%s", phase, err, output)
			}
		}
		mu.Lock()
		defer mu.Unlock()
		for _, text := range []string{"initial before restart", "persisted first", "persisted second"} {
			if counts[text] != 1 {
				t.Errorf("%q executed %d times, want exactly once", text, counts[text])
			}
		}
	})

	t.Run("host process reconnect recovers the original result after a later turn", func(t *testing.T) {
		t.Setenv("ZOT_HOME", t.TempDir())
		socket := filepath.Join(shortSocketDir(t), "host.sock")
		log, err := os.CreateTemp(t.TempDir(), "host-log")
		if err != nil {
			t.Fatal(err)
		}
		defer log.Close()
		host := exec.CommandContext(ctx, binary, "continuous", "serve", "--store", t.TempDir(), "--socket", socket, "--provider", "openai", "--model", "gpt-4o-mini", "--api-key", "synthetic-release-test", "--base-url", server.URL+"/v1", "--no-extensions", "--no-tools")
		host.Stdout, host.Stderr = log, log
		if err := host.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = host.Process.Kill()
			_ = host.Wait()
			if t.Failed() {
				data, _ := os.ReadFile(log.Name())
				t.Logf("host log: %s", data)
			}
		}()
		var observer *continuous.Client
		releaseWait(t, ctx, func() bool {
			observer, err = continuous.Dial(ctx, socket, "")
			return err == nil
		})
		defer observer.Close()
		reconnect := make(chan struct{})
		clients := make(chan *continuous.Client, 4)
		var dials atomic.Int32
		dial := func(ctx context.Context) (*continuous.Client, error) {
			if dials.Add(1) > 1 {
				select {
				case <-reconnect:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			client, err := continuous.Dial(ctx, socket, "")
			if err == nil {
				clients <- client
			}
			return client, err
		}
		f := New(Config{Root: t.TempDir(), RepoRoot: t.TempDir(), NewRunner: NewHostRunnerFactory(dial, "release")})
		a, err := f.SpawnReq(ctx, SpawnRequest{Task: "finish while supervisor disconnected", Model: "gpt-4o-mini", Provider: "openai"})
		if err != nil {
			t.Fatal(err)
		}
		defer stopQueued(t, f, a)
		select {
		case client := <-clients:
			releaseWait(t, ctx, func() bool { return a.Snapshot().Busy })
			_ = client.Close()
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		close(hostRelease)
		child := a.Snapshot().ConversationID
		releaseWait(t, ctx, func() bool {
			var snap continuous.ConversationSnapshot
			return observer.CallInto(ctx, "conversation.snapshot", map[string]any{"id": child}, &snap) == nil && snap.Run != nil && snap.Run.Phase == "done"
		})
		var later continuous.Submission
		if err := observer.CallInto(ctx, "conversation.submit", map[string]any{"id": child, "content": "later turn while disconnected", "request_id": "later-independent-turn"}, &later); err != nil {
			t.Fatal(err)
		}
		releaseWait(t, ctx, func() bool {
			var sub continuous.Submission
			return observer.CallInto(ctx, "submission.get", map[string]any{"id": later.ID}, &sub) == nil && sub.State == "answered"
		})
		close(reconnect)
		releaseWait(t, ctx, func() bool { _, _, ok := a.InitialTurnResult(); return ok })
		response, turnErr, _ := a.InitialTurnResult()
		if response != "echo: finish while supervisor disconnected" || turnErr != "" {
			t.Fatalf("missed original result after reconnect: response %q, error %q", response, turnErr)
		}
		if count := strings.Count(strings.Join(a.Transcript(), "\n"), "echo: finish while supervisor disconnected"); count != 1 {
			t.Fatalf("original output mirrored %d times", count)
		}
		results := make(chan QueuedTurn, 1)
		if _, err := f.QueueTurn(a.ID, "new request after reconnect", func(turn QueuedTurn) bool { results <- turn; return true }); err != nil {
			t.Fatal(err)
		}
		select {
		case result := <-results:
			if result.State != "completed" || result.Response != "echo: new request after reconnect" {
				t.Fatalf("follow-up after recovery: %+v", result)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		mu.Lock()
		defer mu.Unlock()
		for _, text := range []string{"finish while supervisor disconnected", "later turn while disconnected", "new request after reconnect"} {
			if counts[text] != 1 {
				t.Errorf("%q executed %d times", text, counts[text])
			}
		}
	})

	t.Run("cancel stalled work without dropping manual or automated input", func(t *testing.T) {
		t.Setenv("ZOT_HOME", t.TempDir())
		cfg := releaseProcessConfig(t.TempDir(), binary, server.URL)
		f := New(cfg)
		a, err := f.SpawnReq(ctx, SpawnRequest{Task: "blocked initial", Model: "gpt-4o-mini", Provider: "openai"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { stopQueued(t, f, a) })
		for {
			select {
			case text := <-started:
				if text == "blocked initial" {
					goto blocked
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	blocked:
		if err := f.SendUserTurn(a.ID, "manual while busy"); err != nil {
			t.Fatal(err)
		}
		results := make(chan QueuedTurn, 2)
		for _, text := range []string{"automated first", "automated second"} {
			if _, err := f.QueueTurn(a.ID, text, func(turn QueuedTurn) bool { results <- turn; return true }); err != nil {
				t.Fatal(err)
			}
		}
		if err := f.SendInput(a.ID, "cancel"); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"automated first", "automated second"} {
			select {
			case result := <-results:
				if result.Text != want || result.State != "completed" || result.Response != "echo: "+want {
					t.Fatalf("wrong correlated result: %+v", result)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
		if _, turnErr, recorded := a.InitialTurnResult(); !recorded || turnErr == "" {
			t.Fatal("initial cancellation was overwritten by a later reply")
		}
		mu.Lock()
		defer mu.Unlock()
		for _, text := range []string{"blocked initial", "manual while busy", "automated first", "automated second"} {
			if counts[text] != 1 {
				t.Errorf("%q executed %d times", text, counts[text])
			}
		}
	})
}

func releaseProcessConfig(root, binary, endpoint string) Config {
	return Config{Root: filepath.Join(root, "swarm"), RepoRoot: root, NewRunner: func(a *Agent) Runner {
		args := defaultChildArgs(binary, a, a.SessionPath, a.InboxPath)
		args = append(args, "--api-key", "synthetic-release-test", "--base-url", endpoint+"/v1", "--no-extensions", "--no-tools")
		return &execRunner{agent: a, Command: args}
	}}
}

// Run only in the subprocess launched above, never against user state.
func TestSwarmReleaseSupervisorProcess(t *testing.T) {
	phase := os.Getenv("ZOT_RELEASE_PHASE")
	if phase == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	root := os.Getenv("ZOT_RELEASE_ROOT")
	cfg := releaseProcessConfig(root, os.Getenv("ZOT_RELEASE_BINARY"), os.Getenv("ZOT_RELEASE_URL"))
	results := make(chan QueuedTurn, 4)
	cfg.OnQueueResult = func(_ *Agent, turn QueuedTurn) bool { results <- turn; return true }
	f := New(cfg)
	if phase == "accept" {
		a, err := f.SpawnReq(ctx, SpawnRequest{Task: "initial before restart", Model: "gpt-4o-mini", Provider: "openai"})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan string, 1)
		a.SetOnTurnEnd(func(_ int, errMsg string) { done <- errMsg })
		select {
		case errMsg := <-done:
			if errMsg != "" {
				t.Fatal(errMsg)
			}
		case <-ctx.Done():
			t.Fatalf("initial task: %v, snapshot: %+v", ctx.Err(), a.Snapshot())
		}
		stopQueued(t, f, a)
		for _, text := range []string{"persisted first", "persisted second"} {
			if _, err := f.QueueTurn(a.ID, text, nil); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	if count, errs := f.Reload(); count != 1 || len(errs) != 0 {
		t.Fatalf("reload: %d, %v", count, errs)
	}
	a, err := f.Resume(ctx, f.List()[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer stopQueued(t, f, a)
	for _, text := range []string{"persisted first", "persisted second"} {
		select {
		case result := <-results:
			if result.Text != text || result.State != "completed" || result.Response != "echo: "+text {
				t.Fatalf("resumed result: %+v", result)
			}
		case <-ctx.Done():
			t.Fatalf("resumed task: %v, snapshot: %+v", ctx.Err(), a.Snapshot())
		}
	}
}

func releaseWait(t *testing.T, ctx context.Context, ready func() bool) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for !ready() {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-deadline.C:
			t.Fatal("timed out waiting for release smoke-test state")
		case <-ticker.C:
		}
	}
}
