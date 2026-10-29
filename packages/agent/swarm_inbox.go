package agent

import (
	"context"
	"strings"

	"github.com/patriceckhart/zot/packages/agent/swarm"
)

// One scheduler owns the FIFO and cancellation. Manual and automated input
// share it, so neither can race or cause the other to be dropped as busy.
func serveSwarmPrompts(ctx context.Context, initial swarm.Prompt, lines <-chan string, emit func(string, map[string]any), run func(context.Context, swarm.Prompt) (string, error)) error {
	var pending []swarm.Prompt
	if initial.Text != "" || len(initial.Images) > 0 {
		pending = append(pending, initial)
	}
	var cancelFn context.CancelFunc
	turnDone := make(chan struct{}, 1)
	turnNo := 0
	defer func() {
		if cancelFn != nil {
			cancelFn()
			<-turnDone
		}
	}()
	for {
		if ctx.Err() != nil {
			emit("agent_stopped", map[string]any{"reason": "cancelled"})
			return ctx.Err()
		}
		if cancelFn == nil && len(pending) > 0 {
			prompt := pending[0]
			pending = pending[1:]
			turnNo++
			step := turnNo
			turnCtx, turnCancel := context.WithCancel(ctx)
			cancelFn = turnCancel
			emit("turn_start", map[string]any{"step": step, "request_id": prompt.RequestID})
			go func() {
				defer turnCancel()
				response, err := run(turnCtx, prompt)
				emit("turn_end", map[string]any{"step": step, "request_id": prompt.RequestID, "error": errString(err), "response": response})
				turnDone <- struct{}{}
			}()
		}
		select {
		case <-ctx.Done():
			emit("agent_stopped", map[string]any{"reason": "cancelled"})
			return ctx.Err()
		case <-turnDone:
			cancelFn()
			cancelFn = nil
		case msg, ok := <-lines:
			if !ok {
				emit("agent_stopped", map[string]any{"reason": "inbox-closed"})
				return nil
			}
			switch {
			case msg == "shutdown":
				emit("agent_stopped", map[string]any{"reason": "shutdown"})
				return nil
			case msg == "cancel":
				if cancelFn != nil {
					cancelFn()
				}
			case strings.HasPrefix(msg, "user ") || strings.HasPrefix(msg, "user-prompt "):
				prompt, err := swarm.DecodePrompt(msg)
				if err != nil {
					emit("error", map[string]any{"message": err.Error()})
					continue
				}
				pending = append(pending, prompt)
			default:
				// Control payloads can contain private data. Do not echo invalid frames.
				emit("error", map[string]any{"message": "unknown supervisor message"})
			}
		}
	}
}
