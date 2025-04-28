package server_configuration

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

type KeyManager interface {
	PrepareKeys() error
}

const (
	pubEnvVar  = "ED25519_PUBLIC_KEY"
	privEnvVar = "ED25519_PRIVATE_KEY"
)

type Ed25519KeyManager struct {
	config *Configuration
	store  ServerConfigurationManager
}

func NewEd25519KeyManager(cfg *Configuration, store ServerConfigurationManager) KeyManager {
	return &Ed25519KeyManager{config: cfg, store: store}
}

func (m *Ed25519KeyManager) PrepareKeys() error {
	if m.hasConfigKeys() {
		return nil
	}
	if err := m.tryEnvKeys(); err == nil {
		return nil
	}
	return m.generateAndStore()
}

func (m *Ed25519KeyManager) hasConfigKeys() bool {
	return len(m.config.Ed25519PublicKey) > 0 && len(m.config.Ed25519PrivateKey) > 0
}

func (m *Ed25519KeyManager) tryEnvKeys() error {
	pubB64 := os.Getenv(pubEnvVar)
	privB64 := os.Getenv(privEnvVar)
	if pubB64 == "" || privB64 == "" {
		return fmt.Errorf("env keys missing")
	}
	pub, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil {
		return fmt.Errorf("decode public key: %w", err)
	}
	priv, err := base64.StdEncoding.DecodeString(privB64)
	if err != nil {
		return fmt.Errorf("decode private key: %w", err)
	}
	return m.store.InjectEdKeys(ed25519.PublicKey(pub), ed25519.PrivateKey(priv))
}

func (m *Ed25519KeyManager) generateAndStore() error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	return m.store.InjectEdKeys(pub, priv)
}
