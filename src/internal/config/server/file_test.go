package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/curve25519"
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

func testX25519KeyPair(t *testing.T, seed byte) ([]byte, []byte) {
	t.Helper()
	private := make([]byte, 32)
	private[0] = seed * 8
	public, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	return public, private
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

func TestFileMutationPersistsEnvironmentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server_configuration.json")
	configuration := newConfiguration()
	configuration.Host = "configured.example"
	configuration.AllowedPeers = []AllowedPeer{{
		Name: "one", PublicKey: make([]byte, 32), Enabled: true, ClientID: 1,
	}}
	writeServerConfiguration(t, path, *configuration)
	t.Setenv("Host", "environment.example")
	t.Setenv("EnableTCP", "true")

	if err := NewFile(path).SetPeerEnabled(1, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted Configuration
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Host != "environment.example" || !persisted.EnableTCP {
		t.Fatalf("persisted configuration does not contain environment overrides: %+v", persisted)
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
	t.Run("invalid configuration", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server_configuration.json")
		if err := os.WriteFile(path, []byte(`{"Host":" "}`), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewFile(path).Load(); err == nil || !strings.Contains(err.Error(), "host is empty") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("cannot write default", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server_configuration.json") + string(os.PathSeparator)
		if _, err := NewFile(path).Load(); err == nil || !strings.Contains(err.Error(), "could not write default configuration") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestFileOperationsPropagateLoadErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server_configuration.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	file := NewFile(path)

	if err := file.EnsureKeys(); err == nil {
		t.Fatal("EnsureKeys() ignored a load error")
	}
	if _, err := file.Peers(); err == nil {
		t.Fatal("Peers() ignored a load error")
	}
	if err := file.SetPeerEnabled(1, false); err == nil {
		t.Fatal("SetPeerEnabled() ignored a load error")
	}
}

func TestFileEnsureKeys(t *testing.T) {
	t.Run("uses environment", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server_configuration.json")
		writeServerConfiguration(t, path, *newConfiguration())
		public, private := testX25519KeyPair(t, 1)
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
		if !bytes.Equal(configuration.X25519PublicKey, public) ||
			!bytes.Equal(configuration.X25519PrivateKey, private) {
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
		public, private := testX25519KeyPair(t, 1)
		configuration.X25519PublicKey = public
		configuration.X25519PrivateKey = private
		writeServerConfiguration(t, path, *configuration)
		environmentPublic, environmentPrivate := testX25519KeyPair(t, 2)
		t.Setenv(publicKeyEnvVar, base64.StdEncoding.EncodeToString(environmentPublic))
		t.Setenv(privateKeyEnvVar, base64.StdEncoding.EncodeToString(environmentPrivate))

		file := NewFile(path)
		if err := file.EnsureKeys(); err != nil {
			t.Fatal(err)
		}
		loaded, err := file.Load()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(loaded.X25519PublicKey, public) ||
			!bytes.Equal(loaded.X25519PrivateKey, private) {
			t.Fatal("existing keys were replaced")
		}
	})

	t.Run("rejects mismatched existing keys", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server_configuration.json")
		configuration := newConfiguration()
		configuration.X25519PublicKey, _ = testX25519KeyPair(t, 1)
		_, configuration.X25519PrivateKey = testX25519KeyPair(t, 2)
		writeServerConfiguration(t, path, *configuration)

		err := NewFile(path).EnsureKeys()
		if err == nil || !strings.Contains(err.Error(), "public key does not match private key") {
			t.Fatalf("EnsureKeys() error = %v", err)
		}
	})

	t.Run("rejects mismatched environment keys", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "server_configuration.json")
		writeServerConfiguration(t, path, *newConfiguration())
		public, _ := testX25519KeyPair(t, 1)
		_, private := testX25519KeyPair(t, 2)
		t.Setenv(publicKeyEnvVar, base64.StdEncoding.EncodeToString(public))
		t.Setenv(privateKeyEnvVar, base64.StdEncoding.EncodeToString(private))

		err := NewFile(path).EnsureKeys()
		if err == nil || !strings.Contains(err.Error(), "public key does not match private key") {
			t.Fatalf("EnsureKeys() error = %v", err)
		}
	})
}

func TestValidateX25519KeyPairRejectsInvalidPrivateKey(t *testing.T) {
	err := validateX25519KeyPair(make([]byte, 32), []byte{1})
	if err == nil || !strings.Contains(err.Error(), "invalid X25519 private key") {
		t.Fatalf("validateX25519KeyPair() error = %v", err)
	}
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
	if err := file.SetPeerEnabled(2, false); err == nil {
		t.Fatal("missing peer was enabled")
	}
	if err := file.RemovePeer(0); err == nil {
		t.Fatal("invalid client ID was removed")
	}
	if err := file.RemovePeer(2); err == nil {
		t.Fatal("missing peer was removed")
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
