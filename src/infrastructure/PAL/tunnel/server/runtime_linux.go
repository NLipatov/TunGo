package server

import (
	"fmt"
	appConfiguration "tungo/application/configuration"
	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/tunnel/session"
)

func NewRuntime(conf appConfiguration.ServerRuntimeConfiguration) (*Runtime, error) {
	cookieManager, err := noise.NewCookieManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie manager: %w", err)
	}

	return &Runtime{
		sessionRevoker: session.NewCompositeSessionRevoker(),
		allowedPeers:   noise.NewAllowedPeersLookup(conf.AllowedPeers),
		cookieManager:  cookieManager,
		loadMonitor:    noise.NewLoadMonitor(noise.DefaultLoadThreshold),
	}, nil
}
