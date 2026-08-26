package server

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tungo/internal/config/internal/configpath"

	"golang.org/x/crypto/curve25519"
)

const (
	publicKeyEnvVar  = "X25519_PUBLIC_KEY"
	privateKeyEnvVar = "X25519_PRIVATE_KEY"
)

// File owns the persisted server configuration and its mutations.
type File struct {
	path string
}

func DefaultFile() *File {
	return NewFile(filepath.Join(configpath.Directory(), "server_configuration.json"))
}

func NewFile(path string) *File {
	return &File{path: path}
}

func (f *File) Path() string {
	return f.path
}

func (f *File) Load() (*Configuration, error) {
	return f.load()
}

func (f *File) load() (*Configuration, error) {
	configuration, err := f.read()
	if err == nil {
		return configuration, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	configuration = newConfiguration()
	if err := f.write(*configuration); err != nil {
		return nil, fmt.Errorf("could not write default configuration: %w", err)
	}
	return f.read()
}

func (f *File) read() (*Configuration, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("configuration file %q does not exist: %w", f.path, err)
		}
		return nil, fmt.Errorf("configuration file %q is unreadable: %w", f.path, err)
	}

	type configurationWithDeprecatedFields struct {
		Configuration
		FallbackServerAddress string `json:"FallbackServerAddress"`
	}
	var persisted configurationWithDeprecatedFields
	if err := json.Unmarshal(data, &persisted); err != nil {
		return nil, fmt.Errorf("configuration file %q is invalid: %w", f.path, err)
	}

	configuration := persisted.Configuration
	if strings.TrimSpace(configuration.Host) == "" && strings.TrimSpace(persisted.FallbackServerAddress) != "" {
		configuration.Host = strings.TrimSpace(persisted.FallbackServerAddress)
	}
	applyEnvironment(&configuration)
	configuration.applyDefaults()
	if err := validate(configuration); err != nil {
		return nil, fmt.Errorf("configuration file %q is invalid: %w", f.path, err)
	}
	return &configuration, nil
}

func applyEnvironment(configuration *Configuration) {
	host := strings.TrimSpace(os.Getenv("Host"))
	if host == "" {
		host = strings.TrimSpace(os.Getenv("ServerIP"))
	}
	if host != "" {
		configuration.Host = host
	}
	applyBoolEnvironment("EnableUDP", &configuration.EnableUDP)
	applyBoolEnvironment("EnableTCP", &configuration.EnableTCP)
	applyBoolEnvironment("EnableWS", &configuration.EnableWS)
}

func applyBoolEnvironment(name string, destination *bool) {
	value := os.Getenv(name)
	if value == "" {
		return
	}
	if parsed, err := strconv.ParseBool(value); err == nil {
		*destination = parsed
	}
}

func (f *File) write(configuration Configuration) error {
	data, err := json.MarshalIndent(configuration, "", "\t")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0700); err != nil {
		return err
	}
	return os.WriteFile(f.path, data, 0600)
}

func (f *File) update(update func(*Configuration) error) error {
	configuration, err := f.load()
	if err != nil {
		return err
	}
	if err := update(configuration); err != nil {
		return err
	}
	return f.write(*configuration)
}

func (f *File) EnsureKeys() error {
	configuration, err := f.load()
	if err != nil {
		return err
	}
	if len(configuration.X25519PublicKey) == 32 && len(configuration.X25519PrivateKey) == 32 {
		return validateX25519KeyPair(configuration.X25519PublicKey, configuration.X25519PrivateKey)
	}

	public, private, err := configuredOrGeneratedKeys()
	if err != nil {
		return err
	}
	defer clear(public)
	defer clear(private)
	if err := validateX25519KeyPair(public, private); err != nil {
		return err
	}
	configuration.X25519PublicKey = append([]byte(nil), public...)
	configuration.X25519PrivateKey = append([]byte(nil), private...)
	return f.write(*configuration)
}

func configuredOrGeneratedKeys() ([]byte, []byte, error) {
	publicValue := os.Getenv(publicKeyEnvVar)
	privateValue := os.Getenv(privateKeyEnvVar)
	if publicValue != "" && privateValue != "" {
		public, publicErr := base64.StdEncoding.DecodeString(publicValue)
		private, privateErr := base64.StdEncoding.DecodeString(privateValue)
		if publicErr == nil && privateErr == nil && len(public) == 32 && len(private) == 32 {
			return public, private, nil
		}
		clear(public)
		clear(private)
	}

	private := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, private); err != nil {
		clear(private)
		return nil, nil, fmt.Errorf("failed to generate X25519 private key: %w", err)
	}
	public, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil {
		clear(private)
		return nil, nil, fmt.Errorf("failed to derive X25519 public key: %w", err)
	}
	return public, private, nil
}

func validateX25519KeyPair(public, private []byte) error {
	derivedPublic, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil {
		return fmt.Errorf("invalid X25519 private key: %w", err)
	}
	if !bytes.Equal(public, derivedPublic) {
		return fmt.Errorf("X25519 public key does not match private key")
	}
	return nil
}

func (f *File) Peers() ([]AllowedPeer, error) {
	configuration, err := f.Load()
	if err != nil {
		return nil, err
	}
	peers := make([]AllowedPeer, len(configuration.AllowedPeers))
	for i, peer := range configuration.AllowedPeers {
		peers[i] = AllowedPeer{
			Name:      peer.Name,
			PublicKey: append([]byte(nil), peer.PublicKey...),
			Enabled:   peer.Enabled,
			ClientID:  peer.ClientID,
		}
	}
	return peers, nil
}

func (f *File) SetPeerEnabled(clientID int, enabled bool) error {
	if clientID <= 0 {
		return fmt.Errorf("invalid client id %d", clientID)
	}
	return f.update(func(configuration *Configuration) error {
		for i := range configuration.AllowedPeers {
			if configuration.AllowedPeers[i].ClientID == clientID {
				configuration.AllowedPeers[i].Enabled = enabled
				return nil
			}
		}
		return fmt.Errorf("allowed peer with ClientID %d not found", clientID)
	})
}

func (f *File) RemovePeer(clientID int) error {
	if clientID <= 0 {
		return fmt.Errorf("invalid client id %d", clientID)
	}
	return f.update(func(configuration *Configuration) error {
		for i := range configuration.AllowedPeers {
			if configuration.AllowedPeers[i].ClientID == clientID {
				configuration.AllowedPeers = append(configuration.AllowedPeers[:i], configuration.AllowedPeers[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("allowed peer with ClientID %d not found", clientID)
	})
}
