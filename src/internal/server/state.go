package server

import (
	"io"
	"sync"
	"sync/atomic"

	"tungo/internal/config"
	serverconfig "tungo/internal/config/server"
	"tungo/internal/config/settings"
	"tungo/internal/protocol/noise"
	"tungo/internal/server/session"
)

type tunManager interface {
	OpenTunnel(settings.Settings) (io.ReadWriteCloser, error)
	CloseTunnel(settings.Settings) error
}

// Server owns the shared state of all server tunnels.
type Server struct {
	configuration *serverconfig.Configuration
	tunManager    tunManager
	control       config.ServerRuntimeControl
	ready         atomic.Bool
	allowedPeers  *allowedPeers
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
func (r *Server) Update(peers []serverconfig.AllowedPeer) {
	if r.allowedPeers != nil {
		r.allowedPeers.Update(peers)
	}
}
