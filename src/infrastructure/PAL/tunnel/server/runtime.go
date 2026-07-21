package server

import (
	appConfiguration "tungo/application/configuration"
	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/tunnel/session"
)

// Runtime holds shared state used across server workers and configuration updates.
type Runtime struct {
	sessionRevoker *session.CompositeSessionRevoker
	allowedPeers   noise.AllowedPeersLookup
	cookieManager  *noise.CookieManager
	loadMonitor    *noise.LoadMonitor
}

func (r *Runtime) SessionRevoker() *session.CompositeSessionRevoker {
	return r.sessionRevoker
}

func (r *Runtime) AllowedPeersUpdater() appConfiguration.ServerAllowedPeersUpdater {
	if r.allowedPeers == nil {
		return nil
	}
	return r.allowedPeers
}
