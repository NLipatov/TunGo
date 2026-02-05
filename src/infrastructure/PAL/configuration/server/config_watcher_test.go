package server

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockRevoker tracks revoked public keys for testing.
type mockRevoker struct {
	mu      sync.Mutex
	revoked [][]byte
}

func (r *mockRevoker) RevokeByPubKey(pubKey []byte) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	keyCopy := make([]byte, len(pubKey))
	copy(keyCopy, pubKey)
	r.revoked = append(r.revoked, keyCopy)
	return 1
}

func (r *mockRevoker) revokedKeys() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.revoked
}

// mockConfigManager returns configurable AllowedPeers for testing.
type mockConfigManager struct {
	mu     sync.Mutex
	config *Configuration
}

func (m *mockConfigManager) Configuration() (*Configuration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.config, nil
}

func (m *mockConfigManager) IncrementClientCounter() error {
	return nil
}

func (m *mockConfigManager) InjectX25519Keys(_, _ []byte) error {
	return nil
}

func (m *mockConfigManager) setConfig(c *Configuration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = c
}

func TestConfigWatcher_RevokesDisabledPeer(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1
	pubKey2 := make([]byte, 32)
	pubKey2[0] = 2

	initialConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: true, ClientIP: "10.0.0.1"},
			{PublicKey: pubKey2, Enabled: true, ClientIP: "10.0.0.2"},
		},
	}

	configManager := &mockConfigManager{config: initialConfig}
	revoker := &mockRevoker{}

	watcher := NewConfigWatcher(configManager, revoker, 10*time.Millisecond, nil)

	// Initialize state
	watcher.loadCurrentState()

	// Disable peer1
	updatedConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: false, ClientIP: "10.0.0.1"}, // disabled
			{PublicKey: pubKey2, Enabled: true, ClientIP: "10.0.0.2"},
		},
	}
	configManager.setConfig(updatedConfig)

	// Force check
	watcher.ForceCheck()

	// Verify peer1 was revoked
	revoked := revoker.revokedKeys()
	if len(revoked) != 1 {
		t.Fatalf("expected 1 revoked key, got %d", len(revoked))
	}
	if revoked[0][0] != 1 {
		t.Error("expected pubKey1 to be revoked")
	}
}

func TestConfigWatcher_RevokesRemovedPeer(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1
	pubKey2 := make([]byte, 32)
	pubKey2[0] = 2

	initialConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: true, ClientIP: "10.0.0.1"},
			{PublicKey: pubKey2, Enabled: true, ClientIP: "10.0.0.2"},
		},
	}

	configManager := &mockConfigManager{config: initialConfig}
	revoker := &mockRevoker{}

	watcher := NewConfigWatcher(configManager, revoker, 10*time.Millisecond, nil)
	watcher.loadCurrentState()

	// Remove peer1 entirely
	updatedConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey2, Enabled: true, ClientIP: "10.0.0.2"},
		},
	}
	configManager.setConfig(updatedConfig)

	watcher.ForceCheck()

	revoked := revoker.revokedKeys()
	if len(revoked) != 1 {
		t.Fatalf("expected 1 revoked key, got %d", len(revoked))
	}
	if revoked[0][0] != 1 {
		t.Error("expected pubKey1 to be revoked")
	}
}

func TestConfigWatcher_NoRevokeForAlreadyDisabled(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1

	// Start with already disabled peer
	initialConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: false, ClientIP: "10.0.0.1"},
		},
	}

	configManager := &mockConfigManager{config: initialConfig}
	revoker := &mockRevoker{}

	watcher := NewConfigWatcher(configManager, revoker, 10*time.Millisecond, nil)
	watcher.loadCurrentState()

	// Keep disabled
	watcher.ForceCheck()

	// No revocation should happen - was already disabled
	revoked := revoker.revokedKeys()
	if len(revoked) != 0 {
		t.Fatalf("expected 0 revoked keys, got %d", len(revoked))
	}
}

func TestConfigWatcher_NoRevokeWhenReEnabled(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1

	// Start with enabled peer
	initialConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: true, ClientIP: "10.0.0.1"},
		},
	}

	configManager := &mockConfigManager{config: initialConfig}
	revoker := &mockRevoker{}

	watcher := NewConfigWatcher(configManager, revoker, 10*time.Millisecond, nil)
	watcher.loadCurrentState()

	// No change
	watcher.ForceCheck()

	revoked := revoker.revokedKeys()
	if len(revoked) != 0 {
		t.Fatalf("expected 0 revoked keys, got %d", len(revoked))
	}
}

func TestConfigWatcher_WatchLoop(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1

	initialConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: true, ClientIP: "10.0.0.1"},
		},
	}

	configManager := &mockConfigManager{config: initialConfig}
	revoker := &mockRevoker{}

	watcher := NewConfigWatcher(configManager, revoker, 50*time.Millisecond, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watching in background
	go watcher.Watch(ctx)

	// Give time for initialization
	time.Sleep(20 * time.Millisecond)

	// Disable the peer
	configManager.setConfig(&Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: false, ClientIP: "10.0.0.1"},
		},
	})

	// Wait for check interval
	time.Sleep(100 * time.Millisecond)

	// Should have revoked
	revoked := revoker.revokedKeys()
	if len(revoked) != 1 {
		t.Fatalf("expected 1 revoked key after watch loop, got %d", len(revoked))
	}

	cancel()
}
