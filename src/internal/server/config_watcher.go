package server

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	serverconfig "tungo/internal/config/server"

	"github.com/fsnotify/fsnotify"
)

type peerRuntime interface {
	RevokeByPubKey(pubKey []byte) int
	Update(peers []serverconfig.AllowedPeer)
}

const defaultWatchInterval = 30 * time.Second

// configWatcher monitors AllowedPeers configuration changes and:
// 1. Revokes sessions for peers that are removed or disabled
// 2. Updates the runtime AllowedPeers map for new peer lookups
//
// Uses fsnotify for instant updates, with polling as fallback.
type configWatcher struct {
	file     *serverconfig.File
	runtime  peerRuntime
	interval time.Duration
}

type peerAccessState struct {
	enabled  bool
	clientID int
}

// newConfigWatcher creates a watcher for file with the given polling interval.
func newConfigWatcher(
	file *serverconfig.File,
	runtime peerRuntime,
	interval time.Duration,
) *configWatcher {
	return &configWatcher{
		file:     file,
		runtime:  runtime,
		interval: interval,
	}
}

// watch starts the configuration watcher loop.
// Uses fsnotify for instant updates, with polling as fallback.
// Blocks until context is cancelled.
func (w *configWatcher) watch(ctx context.Context) {
	prevPeers := w.loadCurrentState()

	// Try to set up fsnotify - watch directory because atomic writes
	// (write to temp, then rename) lose the watch on the original inode.
	var fsEvents <-chan fsnotify.Event
	var fsErrors <-chan error
	var configFileName string
	watcher, err := fsnotify.NewWatcher()
	if err == nil && w.file.Path() != "" {
		defer func(watcher *fsnotify.Watcher) {
			_ = watcher.Close()
		}(watcher)
		dir, file := filepath.Split(w.file.Path())
		if dir == "" {
			dir = "."
		}
		configFileName = file
		if err := watcher.Add(dir); err == nil {
			fsEvents = watcher.Events
			fsErrors = watcher.Errors
			slog.Info("ConfigWatcher watching directory", "directory", dir, "file", file)
		} else {
			slog.Warn("ConfigWatcher fsnotify watch failed; using polling", "err", err)
		}
	} else if err != nil {
		slog.Warn("ConfigWatcher fsnotify unavailable; using polling", "err", err)
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-fsEvents:
			if !ok {
				fsEvents = nil // Channel closed, fall back to polling only
				continue
			}
			// Filter for our config file only
			_, eventFile := filepath.Split(event.Name)
			if eventFile != configFileName {
				continue
			}
			// Watch for Write, Create, and Rename (atomic writes use rename)
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
				slog.Info("ConfigWatcher detected config change", "op", event.Op)
				prevPeers = w.checkAndRevoke(prevPeers)
			}
		case err, ok := <-fsErrors:
			if !ok {
				fsErrors = nil
				continue
			}
			slog.Warn("ConfigWatcher fsnotify error", "err", err)
		case <-ticker.C:
			prevPeers = w.checkAndRevoke(prevPeers)
		}
	}
}

// loadCurrentState returns the current peer access snapshot.
func (w *configWatcher) loadCurrentState() map[string]peerAccessState {
	conf, err := w.file.Load()
	if err != nil {
		slog.Warn("ConfigWatcher failed to load initial config", "err", err)
		return nil
	}

	peers := make(map[string]peerAccessState, len(conf.AllowedPeers))
	for _, peer := range conf.AllowedPeers {
		key := string(peer.PublicKey)
		peers[key] = peerAccessState{
			enabled:  peer.Enabled,
			clientID: peer.ClientID,
		}
	}
	return peers
}

// checkAndRevoke compares current config with previous state and:
// 1. Revokes sessions for peers that were removed or disabled
// 2. Updates the runtime AllowedPeers map for new handshake lookups
func (w *configWatcher) checkAndRevoke(prevPeers map[string]peerAccessState) map[string]peerAccessState {
	conf, err := w.file.Load()
	if err != nil {
		slog.Warn("ConfigWatcher failed to load config", "err", err)
		return prevPeers
	}

	// Build current state map
	currentPeers := make(map[string]peerAccessState, len(conf.AllowedPeers))
	for _, peer := range conf.AllowedPeers {
		key := string(peer.PublicKey)
		currentPeers[key] = peerAccessState{
			enabled:  peer.Enabled,
			clientID: peer.ClientID,
		}
	}

	// Find peers to revoke:
	// 1. Previously existed and enabled, now removed
	// 2. Previously existed and enabled, now disabled
	for pubKeyStr, prevState := range prevPeers {
		if !prevState.enabled {
			continue // Was already disabled, nothing to revoke
		}

		currentState, exists := currentPeers[pubKeyStr]
		shouldRevoke := !exists || !currentState.enabled || currentState.clientID != prevState.clientID

		if shouldRevoke {
			pubKey := []byte(pubKeyStr)
			count := w.runtime.RevokeByPubKey(pubKey)
			if count > 0 {
				slog.Info("ConfigWatcher revoked sessions for peer", "count", count, "reason", "ACL changed/removed/disabled")
			}
		}
	}

	// Update runtime AllowedPeers map (enables new peers to connect without restart)
	w.runtime.Update(conf.AllowedPeers)

	if len(currentPeers) != len(prevPeers) {
		slog.Info("ConfigWatcher AllowedPeers changed", "previous", len(prevPeers), "current", len(currentPeers))
	}

	return currentPeers
}
