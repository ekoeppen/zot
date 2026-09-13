package extensions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/patriceckhart/zot/packages/agent/extproto"
	"github.com/patriceckhart/zot/packages/provider"
)

// MaxMessagesResponseSize limits the encoded messages value (4 MiB max payload).
const MaxMessagesResponseSize = 4 * 1024 * 1024

// InterceptBeforeModelRequest chains dynamic message transformations in load order.
// Failures are advisory and preserve the current message list.
func (m *Manager) InterceptBeforeModelRequest(ctx context.Context, ev extproto.EventInterceptFromHost, msgs []provider.Message) []provider.Message {
	current := msgs
	ev.Event = "before_model_request"
	if ev.SessionID == "" {
		ev.SessionID = m.sessionID
	}
	if ev.CWD == "" {
		_, ev.CWD = m.SessionContext()
		if ev.CWD == "" {
			ev.CWD = m.cwd
		}
	}
	if ev.Provider == "" {
		ev.Provider = m.provider
	}
	if ev.Model == "" {
		ev.Model = m.model
	}

	m.mu.RLock()
	subs := append([]*Extension(nil), m.extOrder...)
	m.mu.RUnlock()

	for _, ext := range subs {
		if ctx.Err() != nil {
			break
		}
		ext.mu.Lock()
		_, subscribed := ext.interceptSubs[ev.Event]
		ext.mu.Unlock()
		if !subscribed {
			continue
		}

		rawMsgs, err := json.Marshal(current)
		if err != nil {
			continue
		}
		ev.Messages = rawMsgs

		callCtx, cancel := context.WithTimeout(ctx, interceptTimeout)
		r := m.askIntercept(callCtx, ext, ev)
		cancel()

		failure := r.failure
		if failure == "" && len(r.Messages) > 0 {
			switch {
			case len(r.Messages) > MaxMessagesResponseSize:
				failure = "messages exceeds 4 MiB encoded limit"
			default:
				replacement, err := provider.UnmarshalMessages(r.Messages)
				if err != nil {
					failure = fmt.Sprintf("invalid messages JSON: %v", err)
				} else {
					current = replacement
				}
			}
		}

		if failure != "" {
			message := "before_model_request: " + failure + "; keeping current messages"
			fmt.Fprintln(ext.logFile, "[zot] "+message)
			ext.recordDiagnostic(message)
			if m.hooks != nil {
				m.hooks.Notify(ext.Manifest.Name, "warn", message)
			}
		}
	}
	return current
}
