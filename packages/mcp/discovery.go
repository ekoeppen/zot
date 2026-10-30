package mcp

import (
	"context"
	"sync"
)

// discovery owns initial interactive connection attempts. Workers only publish
// completed connections, never call the host under a lifecycle lock, and are
// joined before a reload or shutdown can replace their configuration.
type discovery struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// StartDiscovery reads configuration and starts connecting in the background.
// Each completed server becomes visible independently. Connection failures are
// recorded in Snapshot, while configuration errors are also returned here.
// Hosts must call RefreshStale before a prompt to wait cancellably for initial
// discovery and apply its catalog. Non-interactive hosts should use Reload.
func (m *Manager) StartDiscovery(ctx context.Context) []error {
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()
	m.close()
	m.mu.RLock()
	cwd := m.cwd
	m.mu.RUnlock()
	loaded := loadConfigWithSources(m.home, cwd)
	ctx, cancel := context.WithCancel(ctx)
	d := &discovery{cancel: cancel, done: make(chan struct{})}
	servers := map[string]*server{}
	contexts := map[string]context.Context{}
	var pendingServers []*server
	for name, cfg := range loaded.configs {
		s := &server{name: name, cfg: cfg, raw: loaded.raw[name], source: loaded.sources[name], override: loaded.overrides[name]}
		if !s.disabled() {
			s.connecting = true
			contexts[name], s.cancel = context.WithCancel(ctx)
			pendingServers = append(pendingServers, s)
		}
		servers[name] = s
	}
	m.mu.Lock()
	m.servers, m.invalid, m.invalidSources = servers, loaded.invalid, loaded.sources
	m.fileErrs, m.loaded, m.loadedCWD = loaded.fileErrs, true, cwd
	m.discovery, m.discoveryDirty = d, true
	m.mu.Unlock()

	var wg sync.WaitGroup
	// Do not iterate the published map: workers can already replace entries.
	for _, pending := range pendingServers {
		// The placeholder's cancel also lets a per-server reconnect retire
		// just this attempt, without disturbing other pending servers.
		serverCtx := contexts[pending.name]
		wg.Add(1)
		go func(pending *server) {
			defer wg.Done()
			defer pending.cancel()
			fresh := &server{name: pending.name, cfg: pending.cfg, raw: pending.raw, source: pending.source, override: pending.override}
			_ = m.start(serverCtx, fresh)
			m.mu.Lock()
			current := m.discovery == d && m.servers[pending.name] == pending
			if current {
				m.servers[pending.name] = fresh
				m.rebuildLocked()
				m.discoveryDirty = true
			}
			m.mu.Unlock()
			if current {
				m.activateRelisting(fresh)
			} else {
				fresh.closing.Store(true)
				fresh.shutdown()
			}
		}(pending)
	}
	go func() {
		wg.Wait()
		cancel()
		close(d.done)
	}()
	return loaded.errs
}

// cancelDiscovery is called with lifecycle held. Invalidate publication before
// cancelling, then join without mu held so workers can discard late results.
func (m *Manager) cancelDiscovery() {
	m.mu.Lock()
	d := m.discovery
	m.discovery = nil
	m.mu.Unlock()
	if d != nil {
		d.cancel()
		<-d.done
	}
}

func (m *Manager) waitDiscovery(ctx context.Context) (changed, ready bool) {
	for {
		m.mu.RLock()
		d := m.discovery
		m.mu.RUnlock()
		if d != nil {
			select {
			case <-ctx.Done():
				return false, false
			case <-d.done:
			}
		}
		if ctx.Err() != nil {
			return false, false
		}
		m.mu.Lock()
		if m.discovery != d {
			m.mu.Unlock()
			continue
		}
		changed, m.discoveryDirty = m.discoveryDirty, false
		m.mu.Unlock()
		return changed, true
	}
}
