package agent

import (
	"context"
	"crypto/rand"

	"github.com/patriceckhart/zot/packages/agent/extensions"
	"github.com/patriceckhart/zot/packages/agent/extproto"
	"github.com/patriceckhart/zot/packages/core"
	"github.com/patriceckhart/zot/packages/provider"
)

func wireBeforeModelRequest(ag *core.Agent, mgr *extensions.Manager, providerName string) {
	runtimeSession := rand.Text()
	ag.BeforeModelRequest = func(ctx context.Context, msgs []provider.Message) []provider.Message {
		session := ag.SessionID
		if session == "" {
			session, _ = mgr.SessionContext()
			if session == "" {
				session = runtimeSession
			}
		}
		return mgr.InterceptBeforeModelRequest(ctx, extproto.EventInterceptFromHost{
			SessionID: session,
			Provider:  providerName,
			Model:     ag.Model,
		}, msgs)
	}
}
