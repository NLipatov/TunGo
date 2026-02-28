package server

import (
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/tunnel/session"
)

// Runtime holds shared state used across server workers and the config watcher.
// Created once at startup and passed to both WorkerFactory and ConfigWatcher.
type Runtime struct {
	sessionRevoker *session.CompositeSessionRevoker
	allowedPeers   noise.AllowedPeersLookup
	cookieManager  *noise.CookieManager
	loadMonitor    *noise.LoadMonitor
}

func (r *Runtime) SessionRevoker() *session.CompositeSessionRevoker {
	return r.sessionRevoker
}

func (r *Runtime) AllowedPeersUpdater() server.AllowedPeersUpdater {
	if r.allowedPeers == nil {
		return nil
	}
	return r.allowedPeers
}
