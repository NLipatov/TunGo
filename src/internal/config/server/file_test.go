package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeServerConfiguration(t *testing.T, path string, configuration Configuration) {
	t.Helper()
	data, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestFileLoadCreatesDefaultConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "server_configuration.json")
	configuration, err := NewFile(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if !configuration.EnableUDP || configuration.UDPSettings.TunName == "" {
		t.Fatalf("unexpected default configuration: %+v", configuration)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("default configuration was not written: %v", err)
	}
}

func TestFileLoadAppliesEnvironmentWhenCreatingDefaultConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "server_configuration.json")
	t.Setenv("Host", "env.example")
	t.Setenv("EnableTCP", "true")

	configuration, err := NewFile(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Host != "env.example" || !configuration.EnableTCP {
		t.Fatalf("environment overrides not applied: %+v", configuration)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted Configuration
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Host != "" || persisted.EnableTCP {
		t.Fatalf("environment overrides were persisted: %+v", persisted)
	}
}

func TestFileLoadPreservesCompatibilityAndEnvironmentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server_configuration.json")
	data := []byte(`{"FallbackServerAddress":"old.example","EnableUDP":true}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	configuration, err := NewFile(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Host != "old.example" {
		t.Fatalf("Host = %q", configuration.Host)
	}

	t.Setenv("Host", "env.example")
	t.Setenv("EnableTCP", "true")
	configuration, err = NewFile(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Host != "env.example" || !configuration.EnableTCP {
		t.Fatalf("environment overrides not applied: %+v", configuration)
	}
}

func TestFileLoadErrors(t *testing.T) {
	t.Run("invalid JSON", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server_configuration.json")
		if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewFile(path).Load(); err == nil {
			t.Fatal("expected decode error")
		}
	})
	t.Run("unreadable path", func(t *testing.T) {
		if _, err := NewFile(t.TempDir()).Load(); err == nil || errors.Is(err, os.ErrNotExist) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestFileEnsureKeys(t *testing.T) {
	t.Run("uses environment", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server_configuration.json")
		writeServerConfiguration(t, path, *newConfiguration())
		public := make([]byte, 32)
		private := make([]byte, 32)
		public[0], private[0] = 1, 2
		t.Setenv(publicKeyEnvVar, base64.StdEncoding.EncodeToString(public))
		t.Setenv(privateKeyEnvVar, base64.StdEncoding.EncodeToString(private))
		file := NewFile(path)
		if err := file.EnsureKeys(); err != nil {
			t.Fatal(err)
		}
		configuration, err := file.Load()
		if err != nil {
			t.Fatal(err)
		}
		if configuration.X25519PublicKey[0] != 1 || configuration.X25519PrivateKey[0] != 2 {
			t.Fatal("environment keys were not persisted")
		}
	})

	t.Run("invalid environment falls back to generated keys", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server_configuration.json")
		writeServerConfiguration(t, path, *newConfiguration())
		t.Setenv(publicKeyEnvVar, "invalid")
		t.Setenv(privateKeyEnvVar, "invalid")
		file := NewFile(path)
		if err := file.EnsureKeys(); err != nil {
			t.Fatal(err)
		}
		configuration, err := file.Load()
		if err != nil {
			t.Fatal(err)
		}
		if len(configuration.X25519PublicKey) != 32 || len(configuration.X25519PrivateKey) != 32 {
			t.Fatal("generated keys were not persisted")
		}
	})

	t.Run("preserves existing keys", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server_configuration.json")
		configuration := newConfiguration()
		configuration.X25519PublicKey = append([]byte{3}, make([]byte, 31)...)
		configuration.X25519PrivateKey = append([]byte{4}, make([]byte, 31)...)
		writeServerConfiguration(t, path, *configuration)
		t.Setenv(publicKeyEnvVar, base64.StdEncoding.EncodeToString(make([]byte, 32)))
		t.Setenv(privateKeyEnvVar, base64.StdEncoding.EncodeToString(make([]byte, 32)))

		file := NewFile(path)
		if err := file.EnsureKeys(); err != nil {
			t.Fatal(err)
		}
		loaded, err := file.Load()
		if err != nil {
			t.Fatal(err)
		}
		if loaded.X25519PublicKey[0] != 3 || loaded.X25519PrivateKey[0] != 4 {
			t.Fatal("existing keys were replaced")
		}
	})
}

func TestFilePeerMutationsAndCopies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server_configuration.json")
	configuration := newConfiguration()
	configuration.AllowedPeers = []AllowedPeer{{
		Name: "one", PublicKey: make([]byte, 32), Enabled: true, ClientID: 1,
	}}
	writeServerConfiguration(t, path, *configuration)
	file := NewFile(path)

	peers, err := file.Peers()
	if err != nil {
		t.Fatal(err)
	}
	peers[0].PublicKey[0] = 9
	again, err := file.Peers()
	if err != nil {
		t.Fatal(err)
	}
	if again[0].PublicKey[0] != 0 {
		t.Fatal("Peers returned an aliased public key")
	}

	if err := file.SetPeerEnabled(1, false); err != nil {
		t.Fatal(err)
	}
	if err := file.RemovePeer(1); err != nil {
		t.Fatal(err)
	}
	peers, err = file.Peers()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 0 {
		t.Fatalf("peers = %+v, want none", peers)
	}
	if err := file.SetPeerEnabled(0, true); err == nil {
		t.Fatal("invalid client ID was accepted")
	}
}
