package configuration

import (
	"context"
	"errors"
	"testing"

	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
)

type recordingAllowedPeersUpdater struct {
	peers []ServerPeer
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
		resolver: pathResolverStub{},
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
