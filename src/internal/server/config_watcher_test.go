package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	serverconfig "tungo/internal/config/server"
)

func newWatcherFile(t *testing.T, peers []serverconfig.AllowedPeer) *serverconfig.File {
	t.Helper()
	file := serverconfig.NewFile(filepath.Join(t.TempDir(), "server_configuration.json"))
	setWatcherPeers(t, file, peers)
	return file
}

func setWatcherPeers(t *testing.T, file *serverconfig.File, peers []serverconfig.AllowedPeer) {
	t.Helper()
	configuration, err := file.Load()
	if err != nil {
		t.Fatal(err)
	}
	configuration.AllowedPeers = append([]serverconfig.AllowedPeer(nil), peers...)
	data, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file.Path(), data, 0600); err != nil {
		t.Fatal(err)
	}
}

type recordingPeerRuntime struct {
	keys  [][]byte
	peers []serverconfig.AllowedPeer
}

func (r *recordingPeerRuntime) RevokeByPubKey(key []byte) int {
	r.keys = append(r.keys, append([]byte(nil), key...))
	return 1
}

func (r *recordingPeerRuntime) Update(peers []serverconfig.AllowedPeer) {
	r.peers = append([]serverconfig.AllowedPeer(nil), peers...)
}

func TestConfigWatcherRevokesRemovedAndDisabledPeers(t *testing.T) {
	enabledKey := make([]byte, 32)
	enabledKey[0] = 1
	disabledKey := make([]byte, 32)
	disabledKey[0] = 2
	file := newWatcherFile(t, []serverconfig.AllowedPeer{
		{PublicKey: enabledKey, Enabled: true, ClientID: 1},
		{PublicKey: disabledKey, Enabled: false, ClientID: 2},
	})
	runtime := &recordingPeerRuntime{}
	watcher := newConfigWatcher(file, runtime, time.Hour)
	previous := watcher.loadCurrentState()

	setWatcherPeers(t, file, []serverconfig.AllowedPeer{{PublicKey: disabledKey, Enabled: false, ClientID: 2}})
	current := watcher.checkAndRevoke(previous)
	if len(runtime.keys) != 1 || runtime.keys[0][0] != 1 {
		t.Fatalf("revoked keys = %v", runtime.keys)
	}
	if len(current) != 1 || len(runtime.peers) != 1 {
		t.Fatalf("current=%v updated=%v", current, runtime.peers)
	}
}

func TestConfigWatcherRevokesChangedClientID(t *testing.T) {
	key := make([]byte, 32)
	file := newWatcherFile(t, []serverconfig.AllowedPeer{
		{PublicKey: key, Enabled: true, ClientID: 1},
	})
	runtime := &recordingPeerRuntime{}
	watcher := newConfigWatcher(file, runtime, time.Hour)
	previous := watcher.loadCurrentState()
	setWatcherPeers(t, file, []serverconfig.AllowedPeer{{PublicKey: key, Enabled: true, ClientID: 2}})
	watcher.checkAndRevoke(previous)
	if len(runtime.keys) != 1 {
		t.Fatalf("revocations = %d, want 1", len(runtime.keys))
	}
}

func TestConfigWatcherStopsWithContext(t *testing.T) {
	file := serverconfig.NewFile(filepath.Join(t.TempDir(), "server_configuration.json"))
	watcher := newConfigWatcher(file, &recordingPeerRuntime{}, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		watcher.watch(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher did not stop")
	}
}
