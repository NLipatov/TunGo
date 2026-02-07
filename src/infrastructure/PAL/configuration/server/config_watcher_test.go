package server

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
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
	mu              sync.Mutex
	config          *Configuration
	configErr       error
	invalidateCalls int
}

func (m *mockConfigManager) Configuration() (*Configuration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.configErr != nil {
		return nil, m.configErr
	}
	return m.config, nil
}

func (m *mockConfigManager) IncrementClientCounter() error {
	return nil
}

func (m *mockConfigManager) InjectX25519Keys(_, _ []byte) error {
	return nil
}

func (m *mockConfigManager) AddAllowedPeer(_ AllowedPeer) error {
	return nil
}

func (m *mockConfigManager) InvalidateCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidateCalls++
}

func (m *mockConfigManager) setConfig(c *Configuration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = c
}

func (m *mockConfigManager) setConfigError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configErr = err
}

func (m *mockConfigManager) invalidateCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.invalidateCalls
}

type mockPeersUpdater struct {
	mu      sync.Mutex
	updates [][]AllowedPeer
}

func (u *mockPeersUpdater) Update(peers []AllowedPeer) {
	u.mu.Lock()
	defer u.mu.Unlock()
	cp := make([]AllowedPeer, len(peers))
	copy(cp, peers)
	u.updates = append(u.updates, cp)
}

func (u *mockPeersUpdater) updatesCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.updates)
}

func TestConfigWatcher_RevokesDisabledPeer(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1
	pubKey2 := make([]byte, 32)
	pubKey2[0] = 2

	initialConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: true, Address: netip.MustParseAddr("10.0.0.1")},
			{PublicKey: pubKey2, Enabled: true, Address: netip.MustParseAddr("10.0.0.2")},
		},
	}

	configManager := &mockConfigManager{config: initialConfig}
	revoker := &mockRevoker{}

	watcher := NewConfigWatcher(configManager, revoker, nil, "", 10*time.Millisecond, nil)

	// Initialize state
	watcher.loadCurrentState()

	// Disable peer1
	updatedConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: false, Address: netip.MustParseAddr("10.0.0.1")}, // disabled
			{PublicKey: pubKey2, Enabled: true, Address: netip.MustParseAddr("10.0.0.2")},
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
			{PublicKey: pubKey1, Enabled: true, Address: netip.MustParseAddr("10.0.0.1")},
			{PublicKey: pubKey2, Enabled: true, Address: netip.MustParseAddr("10.0.0.2")},
		},
	}

	configManager := &mockConfigManager{config: initialConfig}
	revoker := &mockRevoker{}

	watcher := NewConfigWatcher(configManager, revoker, nil, "", 10*time.Millisecond, nil)
	watcher.loadCurrentState()

	// Remove peer1 entirely
	updatedConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey2, Enabled: true, Address: netip.MustParseAddr("10.0.0.2")},
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
			{PublicKey: pubKey1, Enabled: false, Address: netip.MustParseAddr("10.0.0.1")},
		},
	}

	configManager := &mockConfigManager{config: initialConfig}
	revoker := &mockRevoker{}

	watcher := NewConfigWatcher(configManager, revoker, nil, "", 10*time.Millisecond, nil)
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
			{PublicKey: pubKey1, Enabled: true, Address: netip.MustParseAddr("10.0.0.1")},
		},
	}

	configManager := &mockConfigManager{config: initialConfig}
	revoker := &mockRevoker{}

	watcher := NewConfigWatcher(configManager, revoker, nil, "", 10*time.Millisecond, nil)
	watcher.loadCurrentState()

	// No change
	watcher.ForceCheck()

	revoked := revoker.revokedKeys()
	if len(revoked) != 0 {
		t.Fatalf("expected 0 revoked keys, got %d", len(revoked))
	}
}

func TestConfigWatcher_RevokesPeerWhenAddressChanged(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1

	initialConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: true, Address: netip.MustParseAddr("10.0.0.10")},
		},
	}

	configManager := &mockConfigManager{config: initialConfig}
	revoker := &mockRevoker{}

	watcher := NewConfigWatcher(configManager, revoker, nil, "", 10*time.Millisecond, nil)
	watcher.loadCurrentState()

	configManager.setConfig(&Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: true, Address: netip.MustParseAddr("10.0.0.11")},
		},
	})

	watcher.ForceCheck()

	revoked := revoker.revokedKeys()
	if len(revoked) != 1 {
		t.Fatalf("expected 1 revoked key, got %d", len(revoked))
	}
	if revoked[0][0] != 1 {
		t.Error("expected pubKey1 to be revoked")
	}
}

func TestConfigWatcher_WatchLoop(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1

	initialConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: true, Address: netip.MustParseAddr("10.0.0.1")},
		},
	}

	configManager := &mockConfigManager{config: initialConfig}
	revoker := &mockRevoker{}

	watcher := NewConfigWatcher(configManager, revoker, nil, "", 50*time.Millisecond, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watching in background
	go watcher.Watch(ctx)

	// Give time for initialization
	time.Sleep(20 * time.Millisecond)

	// Disable the peer
	configManager.setConfig(&Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: false, Address: netip.MustParseAddr("10.0.0.1")},
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

func TestConfigWatcher_CheckAndRevoke_UpdatesPeersUpdater(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1

	initialConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: true, Address: netip.MustParseAddr("10.0.0.1")},
		},
	}

	configManager := &mockConfigManager{config: initialConfig}
	revoker := &mockRevoker{}
	updater := &mockPeersUpdater{}

	watcher := NewConfigWatcher(configManager, revoker, updater, "", 10*time.Millisecond, nil)
	watcher.loadCurrentState()
	watcher.ForceCheck()

	if updater.updatesCount() != 1 {
		t.Fatalf("expected one peers update, got %d", updater.updatesCount())
	}
}

func TestConfigWatcher_LoadAndCheck_ConfigError_NoPanic(t *testing.T) {
	configManager := &mockConfigManager{configErr: errors.New("boom")}
	revoker := &mockRevoker{}

	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)
	watcher := NewConfigWatcher(configManager, revoker, nil, "", 10*time.Millisecond, logger)

	// Should just log and return.
	watcher.loadCurrentState()
	watcher.checkAndRevoke()

	if got := logBuf.String(); got == "" {
		t.Fatal("expected watcher to log config errors")
	}
}

func TestConfigWatcher_Watch_FsnotifyEventTriggersInvalidateAndRevoke(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1
	pubKey2 := make([]byte, 32)
	pubKey2[0] = 2

	initialConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: true, Address: netip.MustParseAddr("10.0.0.1")},
			{PublicKey: pubKey2, Enabled: true, Address: netip.MustParseAddr("10.0.0.2")},
		},
	}

	configManager := &mockConfigManager{config: initialConfig}
	revoker := &mockRevoker{}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "server.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	watcher := NewConfigWatcher(configManager, revoker, nil, configPath, time.Hour, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Watch(ctx)

	time.Sleep(80 * time.Millisecond)

	// Disable peer1 and trigger fs event on watched file.
	configManager.setConfig(&Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: false, Address: netip.MustParseAddr("10.0.0.1")},
			{PublicKey: pubKey2, Enabled: true, Address: netip.MustParseAddr("10.0.0.2")},
		},
	})
	if err := os.WriteFile(configPath, []byte("{\"changed\":true}"), 0o644); err != nil {
		t.Fatalf("write changed file: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(revoker.revokedKeys()) == 1 && configManager.invalidateCount() > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected fsnotify-driven revoke+invalidate, revoked=%d invalidates=%d",
		len(revoker.revokedKeys()), configManager.invalidateCount())
}

func TestConfigWatcher_Watch_InvalidPathFallsBackToPolling(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1

	initialConfig := &Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: true, Address: netip.MustParseAddr("10.0.0.1")},
		},
	}

	configManager := &mockConfigManager{config: initialConfig}
	revoker := &mockRevoker{}
	watcher := NewConfigWatcher(
		configManager,
		revoker,
		nil,
		filepath.Join(t.TempDir(), "missing", "server.json"), // watcher.Add(dir) fails
		40*time.Millisecond,
		nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Watch(ctx)

	time.Sleep(30 * time.Millisecond)
	configManager.setConfig(&Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: false, Address: netip.MustParseAddr("10.0.0.1")},
		},
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(revoker.revokedKeys()) == 1 && configManager.invalidateCount() > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected polling fallback revoke+invalidate, revoked=%d invalidates=%d",
		len(revoker.revokedKeys()), configManager.invalidateCount())
}

func TestConfigWatcher_Watch_LogsWatchDirForBareFilename(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1

	configManager := &mockConfigManager{
		config: &Configuration{
			AllowedPeers: []AllowedPeer{
				{PublicKey: pubKey1, Enabled: true, Address: netip.MustParseAddr("10.0.0.1")},
			},
		},
	}
	revoker := &mockRevoker{}
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.WriteFile("server.json", []byte("{}"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	watcher := NewConfigWatcher(configManager, revoker, nil, "server.json", time.Hour, logger)
	ctx, cancel := context.WithCancel(context.Background())
	go watcher.Watch(ctx)
	time.Sleep(80 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)

	if !bytes.Contains(logBuf.Bytes(), []byte("watching directory . for changes to server.json")) {
		t.Fatalf("expected watch-dir log, got: %s", logBuf.String())
	}
}

func TestConfigWatcher_CheckAndRevoke_LoggerBranches(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1
	pubKey2 := make([]byte, 32)
	pubKey2[0] = 2

	configManager := &mockConfigManager{
		config: &Configuration{
			AllowedPeers: []AllowedPeer{
				{PublicKey: pubKey1, Enabled: true, Address: netip.MustParseAddr("10.0.0.1")},
				{PublicKey: pubKey2, Enabled: true, Address: netip.MustParseAddr("10.0.0.2")},
			},
		},
	}
	revoker := &mockRevoker{}
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)
	watcher := NewConfigWatcher(configManager, revoker, nil, "", 10*time.Millisecond, logger)
	watcher.loadCurrentState()

	// Remove one peer to trigger revoke log (count > 0) and peer-count-changed log.
	configManager.setConfig(&Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey2, Enabled: true, Address: netip.MustParseAddr("10.0.0.2")},
		},
	})
	watcher.checkAndRevoke()

	logs := logBuf.String()
	if !strings.Contains(logs, "revoked 1 session(s)") {
		t.Fatalf("expected revoke log, got: %s", logs)
	}
	if !strings.Contains(logs, "AllowedPeers changed (2 -> 1 peers)") {
		t.Fatalf("expected peer-count-change log, got: %s", logs)
	}
}

func TestConfigWatcher_Watch_LogsFsnotifyAddFailure(t *testing.T) {
	configManager := &mockConfigManager{config: &Configuration{}}
	revoker := &mockRevoker{}
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	// Missing directory -> watcher.Add(dir) fails, should log fallback message.
	watcher := NewConfigWatcher(
		configManager,
		revoker,
		nil,
		filepath.Join(t.TempDir(), "missing", "server.json"),
		20*time.Millisecond,
		logger,
	)

	ctx, cancel := context.WithCancel(context.Background())
	go watcher.Watch(ctx)
	time.Sleep(80 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)

	if !strings.Contains(logBuf.String(), "fsnotify watch failed") {
		t.Fatalf("expected fsnotify watch-failed log, got: %s", logBuf.String())
	}
}

func TestConfigWatcher_Watch_IgnoresOtherFilesAndLogsOwnFile(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1
	pubKey2 := make([]byte, 32)
	pubKey2[0] = 2

	configManager := &mockConfigManager{
		config: &Configuration{
			AllowedPeers: []AllowedPeer{
				{PublicKey: pubKey1, Enabled: true, Address: netip.MustParseAddr("10.0.0.1")},
				{PublicKey: pubKey2, Enabled: true, Address: netip.MustParseAddr("10.0.0.2")},
			},
		},
	}
	revoker := &mockRevoker{}
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "server.json")
	otherPath := filepath.Join(dir, "other.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(otherPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write other: %v", err)
	}

	watcher := NewConfigWatcher(configManager, revoker, nil, configPath, time.Hour, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Watch(ctx)

	time.Sleep(80 * time.Millisecond)

	// Change non-target file: must be ignored (no revoke).
	if err := os.WriteFile(otherPath, []byte("{\"x\":1}"), 0o644); err != nil {
		t.Fatalf("write other changed: %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	if len(revoker.revokedKeys()) != 0 {
		t.Fatalf("expected no revokes from other-file event, got %d", len(revoker.revokedKeys()))
	}

	// Now change target config and state to trigger revoke.
	configManager.setConfig(&Configuration{
		AllowedPeers: []AllowedPeer{
			{PublicKey: pubKey1, Enabled: false, Address: netip.MustParseAddr("10.0.0.1")},
			{PublicKey: pubKey2, Enabled: true, Address: netip.MustParseAddr("10.0.0.2")},
		},
	})
	if err := os.WriteFile(configPath, []byte("{\"changed\":true}"), 0o644); err != nil {
		t.Fatalf("write config changed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(revoker.revokedKeys()) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(revoker.revokedKeys()) != 1 {
		t.Fatalf("expected one revoke from config event, got %d", len(revoker.revokedKeys()))
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "detected config change") {
		t.Fatalf("expected detected-change log, got: %s", logs)
	}
}
