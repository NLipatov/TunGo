package configuration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
)

type recordingAllowedPeersUpdater struct {
	peers []ServerPeer
}

func TestWriteServerClientConfigFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "configuration")
	configPath := filepath.Join(dir, "server_configuration.json")

	path, err := writeServerClientConfigFile(configPath, 7, []byte("client configuration"))
	if err != nil {
		t.Fatalf("writeServerClientConfigFile() error = %v", err)
	}
	if got, want := path, filepath.Join(dir, "client_configuration.json.7"); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	if got, want := string(data), "client configuration"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestWriteServerClientConfigFile_MkdirError(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(file, []byte("occupied"), 0600); err != nil {
		t.Fatalf("create blocking file: %v", err)
	}

	_, err := writeServerClientConfigFile(filepath.Join(file, "server.json"), 1, nil)
	if err == nil || !strings.Contains(err.Error(), "create server config directory") {
		t.Fatalf("expected mkdir error, got %v", err)
	}
}

func TestWriteServerClientConfigFile_WriteError(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "client_configuration.json.3")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatalf("create directory at destination path: %v", err)
	}

	path, err := writeServerClientConfigFile(filepath.Join(dir, "server.json"), 3, nil)
	if err == nil {
		t.Fatal("expected write error for directory destination")
	}
	if path != destination {
		t.Fatalf("path = %q, want %q", path, destination)
	}
}

func (u *recordingAllowedPeersUpdater) Update(peers []ServerPeer) {
	u.peers = peers
}

func TestAllowedPeersUpdater(t *testing.T) {
	allowedPeersUpdater{}.Update(nil)

	key := []byte{1, 2, 3}
	recorder := &recordingAllowedPeersUpdater{}
	allowedPeersUpdater{updater: recorder}.Update([]serverConfiguration.AllowedPeer{{
		Name:      "client-1",
		PublicKey: key,
		Enabled:   true,
		ClientID:  7,
	}})

	if len(recorder.peers) != 1 || recorder.peers[0].Name != "client-1" || recorder.peers[0].ClientID != 7 || !recorder.peers[0].Enabled {
		t.Fatalf("Update() peers = %+v", recorder.peers)
	}
	key[0] = 9
	if recorder.peers[0].PublicKey[0] != 1 {
		t.Fatalf("Update() retained source key: %v", recorder.peers[0].PublicKey)
	}
}

func TestServerControlWatchStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	control := serverControl{
		configPath: "/tmp/server.json",
		manager: &runtimeInfoServerManager{
			cfg: &serverConfiguration.Configuration{},
		},
	}

	control.WatchServerRuntimeConfiguration(ctx, nil, nil)
}

func TestServerControlRuntimeConfiguration_KeyPreparationError(t *testing.T) {
	want := errors.New("inject failed")
	t.Setenv("X25519_PUBLIC_KEY", "")
	t.Setenv("X25519_PRIVATE_KEY", "")
	control := serverControl{
		manager: &runtimeInfoServerManager{
			cfg:       &serverConfiguration.Configuration{},
			injectErr: want,
		},
	}

	_, err := control.ServerRuntimeConfiguration()
	if !errors.Is(err, want) {
		t.Fatalf("ServerRuntimeConfiguration() error = %v, want %v", err, want)
	}
}

func TestServerControlRuntimeConfiguration_ConfigurationError(t *testing.T) {
	want := errors.New("read failed")
	calls := 0
	control := serverControl{
		manager: &runtimeInfoServerManager{
			configuration: func() (*serverConfiguration.Configuration, error) {
				calls++
				if calls == 1 {
					return &serverConfiguration.Configuration{
						X25519PublicKey:  make([]byte, 32),
						X25519PrivateKey: make([]byte, 32),
					}, nil
				}
				return nil, want
			},
		},
	}

	_, err := control.ServerRuntimeConfiguration()
	if !errors.Is(err, want) {
		t.Fatalf("ServerRuntimeConfiguration() error = %v, want %v", err, want)
	}
}

func TestServerControlGenerateClientConfiguration_KeyPreparationError(t *testing.T) {
	want := errors.New("inject failed")
	t.Setenv("X25519_PUBLIC_KEY", "")
	t.Setenv("X25519_PRIVATE_KEY", "")
	control := serverControl{
		manager: &runtimeInfoServerManager{
			cfg:       &serverConfiguration.Configuration{},
			injectErr: want,
		},
	}

	_, err := control.GenerateClientConfiguration()
	if !errors.Is(err, want) {
		t.Fatalf("GenerateClientConfiguration() error = %v, want %v", err, want)
	}
}

func TestServerControlGenerateClientConfiguration(t *testing.T) {
	conf := serverConfiguration.NewDefaultConfiguration()
	conf.FallbackServerAddress = "127.0.0.1"
	conf.X25519PublicKey = make([]byte, 32)
	conf.X25519PrivateKey = make([]byte, 32)
	manager := &runtimeInfoServerManager{cfg: conf}
	dir := t.TempDir()
	control := serverControl{
		configPath: filepath.Join(dir, "server_configuration.json"),
		manager:    manager,
	}

	generated, err := control.GenerateClientConfiguration()
	if err != nil {
		t.Fatalf("GenerateClientConfiguration() error = %v", err)
	}
	if generated.JSON == "" {
		t.Fatal("expected generated JSON")
	}
	if got, want := generated.Path, filepath.Join(dir, "client_configuration.json.1"); got != want {
		t.Fatalf("generated path = %q, want %q", got, want)
	}
	if _, err := os.Stat(generated.Path); err != nil {
		t.Fatalf("stat generated configuration: %v", err)
	}
}
