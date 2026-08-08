package server

import (
	"fmt"
	appConfiguration "tungo/application/configuration"
	"tungo/infrastructure/cryptography/noise"
)

// New builds a server that owns all configured protocol tunnels.
func New(
	conf appConfiguration.ServerRuntimeConfiguration,
	tunManager tunManager,
) (*Server, error) {
	cookieManager, err := noise.NewCookieManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie manager: %w", err)
	}

	return &Server{
		configuration: conf,
		tunManager:    tunManager,
		allowedPeers:  noise.NewAllowedPeersLookup(conf.AllowedPeers),
		cookieManager: cookieManager,
		loadMonitor:   noise.NewLoadMonitor(noise.DefaultLoadThreshold),
	}, nil
}
