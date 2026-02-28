package server

import (
	"fmt"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/tunnel/session"
)

func NewRuntime(manager server.ConfigurationManager) (*Runtime, error) {
	conf, err := manager.Configuration()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

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
