package server

import (
	"io"
	"sync"

	appConfiguration "tungo/application/configuration"
	"tungo/application/configuration/settings"
	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/tunnel/server/internal/session"
)

type tunManager interface {
	CreateDevice(settings.Settings) (io.ReadWriteCloser, error)
	DisposeDevices(settings.Settings) error
}

// Server owns the shared state of all server tunnels.
type Server struct {
	configuration appConfiguration.ServerRuntimeConfiguration
	tunManager    tunManager
	allowedPeers  noise.AllowedPeersLookup
	cookieManager *noise.CookieManager
	loadMonitor   *noise.LoadMonitor

	repositoriesMu sync.RWMutex
	repositories   []*session.Repository
}

func (r *Server) register(repository *session.Repository) {
	r.repositoriesMu.Lock()
	r.repositories = append(r.repositories, repository)
	r.repositoriesMu.Unlock()
}

// RevokeByPubKey terminates matching sessions across every active protocol.
func (r *Server) RevokeByPubKey(publicKey []byte) int {
	r.repositoriesMu.RLock()
	repositories := append([]*session.Repository(nil), r.repositories...)
	r.repositoriesMu.RUnlock()

	total := 0
	for _, repository := range repositories {
		total += repository.TerminateByPubKey(publicKey)
	}
	return total
}

// Update replaces the peers accepted by new handshakes.
func (r *Server) Update(peers []appConfiguration.ServerPeer) {
	if r.allowedPeers != nil {
		r.allowedPeers.Update(peers)
	}
}
