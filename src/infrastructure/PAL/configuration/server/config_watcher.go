package server

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// SessionRevoker revokes sessions by public key.
// Implemented by session.CompositeSessionRevoker.
type SessionRevoker interface {
	RevokeByPubKey(pubKey []byte) int
}

// AllowedPeersUpdater updates the runtime AllowedPeers map.
// Implemented by noise.allowedPeersMap.
type AllowedPeersUpdater interface {
	Update(peers []AllowedPeer)
}

// ConfigWatcher monitors AllowedPeers configuration changes and:
// 1. Revokes sessions for peers that are removed or disabled
// 2. Updates the runtime AllowedPeers map for new peer lookups
//
// Uses fsnotify for instant updates, with polling as fallback.
type ConfigWatcher struct {
	configManager ConfigurationManager
	revoker       SessionRevoker
	peersUpdater  AllowedPeersUpdater
	configPath    string
	interval      time.Duration
	logger        *log.Logger

	// prevPeers stores the previous AllowedPeers state for comparison.
	// Key is string(PublicKey), value is Enabled status.
	prevPeers map[string]bool
}

// NewConfigWatcher creates a new configuration watcher.
// configPath is the path to watch for changes (fsnotify).
// interval is the fallback polling interval (recommend: 30s-60s).
// peersUpdater can be nil if runtime peer updates are not needed.
func NewConfigWatcher(
	configManager ConfigurationManager,
	revoker SessionRevoker,
	peersUpdater AllowedPeersUpdater,
	configPath string,
	interval time.Duration,
	logger *log.Logger,
) *ConfigWatcher {
	return &ConfigWatcher{
		configManager: configManager,
		revoker:       revoker,
		peersUpdater:  peersUpdater,
		configPath:    configPath,
		interval:      interval,
		logger:        logger,
		prevPeers:     make(map[string]bool),
	}
}

// Watch starts the configuration watcher loop.
// Uses fsnotify for instant updates, with polling as fallback.
// Blocks until context is cancelled.
func (w *ConfigWatcher) Watch(ctx context.Context) {
	// Initialize with current state
	w.loadCurrentState()

	// Try to set up fsnotify - watch directory because atomic writes
	// (write to temp, then rename) lose the watch on the original inode.
	var fsEvents <-chan fsnotify.Event
	var fsErrors <-chan error
	var configFileName string
	watcher, err := fsnotify.NewWatcher()
	if err == nil && w.configPath != "" {
		defer watcher.Close()
		dir, file := filepath.Split(w.configPath)
		if dir == "" {
			dir = "."
		}
		configFileName = file
		if err := watcher.Add(dir); err == nil {
			fsEvents = watcher.Events
			fsErrors = watcher.Errors
			if w.logger != nil {
				w.logger.Printf("ConfigWatcher: watching directory %s for changes to %s", dir, file)
			}
		} else if w.logger != nil {
			w.logger.Printf("ConfigWatcher: fsnotify watch failed: %v (using polling)", err)
		}
	} else if w.logger != nil && err != nil {
		w.logger.Printf("ConfigWatcher: fsnotify unavailable: %v (using polling)", err)
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
				if w.logger != nil {
					w.logger.Printf("ConfigWatcher: detected config change (op=%s)", event.Op)
				}
				// Invalidate cache before reading fresh config
				w.configManager.InvalidateCache()
				w.checkAndRevoke()
			}
		case err, ok := <-fsErrors:
			if !ok {
				fsErrors = nil
				continue
			}
			if w.logger != nil {
				w.logger.Printf("ConfigWatcher: fsnotify error: %v", err)
			}
		case <-ticker.C:
			w.checkAndRevoke()
		}
	}
}

// loadCurrentState initializes prevPeers from current configuration.
func (w *ConfigWatcher) loadCurrentState() {
	conf, err := w.configManager.Configuration()
	if err != nil {
		if w.logger != nil {
			w.logger.Printf("ConfigWatcher: failed to load initial config: %v", err)
		}
		return
	}

	w.prevPeers = make(map[string]bool, len(conf.AllowedPeers))
	for _, peer := range conf.AllowedPeers {
		key := string(peer.PublicKey)
		w.prevPeers[key] = peer.Enabled
	}
}

// checkAndRevoke compares current config with previous state and:
// 1. Revokes sessions for peers that were removed or disabled
// 2. Updates the runtime AllowedPeers map for new handshake lookups
func (w *ConfigWatcher) checkAndRevoke() {
	conf, err := w.configManager.Configuration()
	if err != nil {
		if w.logger != nil {
			w.logger.Printf("ConfigWatcher: failed to load config: %v", err)
		}
		return
	}

	// Build current state map
	currentPeers := make(map[string]bool, len(conf.AllowedPeers))
	for _, peer := range conf.AllowedPeers {
		key := string(peer.PublicKey)
		currentPeers[key] = peer.Enabled
	}

	// Find peers to revoke:
	// 1. Previously existed and enabled, now removed
	// 2. Previously existed and enabled, now disabled
	for pubKeyStr, wasEnabled := range w.prevPeers {
		if !wasEnabled {
			continue // Was already disabled, nothing to revoke
		}

		nowEnabled, exists := currentPeers[pubKeyStr]
		shouldRevoke := !exists || !nowEnabled

		if shouldRevoke {
			pubKey := []byte(pubKeyStr)
			count := w.revoker.RevokeByPubKey(pubKey)
			if w.logger != nil && count > 0 {
				w.logger.Printf("ConfigWatcher: revoked %d session(s) for peer (removed/disabled)", count)
			}
		}
	}

	// Update runtime AllowedPeers map (enables new peers to connect without restart)
	if w.peersUpdater != nil {
		w.peersUpdater.Update(conf.AllowedPeers)
	}

	// Log only if peer count changed
	if w.logger != nil && len(currentPeers) != len(w.prevPeers) {
		w.logger.Printf("ConfigWatcher: AllowedPeers changed (%d -> %d peers)", len(w.prevPeers), len(currentPeers))
	}

	// Update previous state
	w.prevPeers = currentPeers
}

// ForceCheck triggers an immediate configuration check.
// Useful for testing or manual triggers (e.g., SIGHUP handler).
func (w *ConfigWatcher) ForceCheck() {
	w.checkAndRevoke()
}
