package server

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/curve25519"
)

type KeyManager interface {
	// PrepareKeys guarantees that X25519 keys are presented in configuration
	PrepareKeys() error
}

const (
	publicKeyEnvVar  = "X25519_PUBLIC_KEY"
	privateKeyEnvVar = "X25519_PRIVATE_KEY"
)

type X25519KeyManager struct {
	configurationManager ConfigurationManager
}

func NewX25519KeyManager(store ConfigurationManager) KeyManager {
	return &X25519KeyManager{
		configurationManager: store,
	}
}

func (m *X25519KeyManager) PrepareKeys() error {
	if keysAreInConfiguration, err := m.keysAreInConfiguration(); keysAreInConfiguration && err == nil {
		return nil
	}

	if keysAreInEnvVariables, err := m.keysAreInEnvVariables(); keysAreInEnvVariables && err == nil {
		return nil
	}

	return m.generateAndStoreKeysInConfiguration()
}

func (m *X25519KeyManager) keysAreInConfiguration() (bool, error) {
	configuration, err := m.configurationManager.Configuration()
	if err != nil {
		return false, err
	}
	pubKeyValid := len(configuration.X25519PublicKey) == 32
	privKeyValid := len(configuration.X25519PrivateKey) == 32
	return pubKeyValid && privKeyValid, nil
}

func (m *X25519KeyManager) keysAreInEnvVariables() (bool, error) {
	publicKeyString := os.Getenv(publicKeyEnvVar)
	privateKeyString := os.Getenv(privateKeyEnvVar)
	if publicKeyString == "" || privateKeyString == "" {
		return false, nil
	}
	publicKey, publicKeyErr := base64.StdEncoding.DecodeString(publicKeyString)
	if publicKeyErr != nil {
		return false, fmt.Errorf("failed to decode public key: %w", publicKeyErr)
	}
	privateKey, privateKeyErr := base64.StdEncoding.DecodeString(privateKeyString)
	if privateKeyErr != nil {
		return false, fmt.Errorf("failed to decode private key: %w", privateKeyErr)
	}
	if len(publicKey) != 32 || len(privateKey) != 32 {
		return false, fmt.Errorf("invalid X25519 key length")
	}
	defer func() {
		for i := range publicKey {
			publicKey[i] = 0
		}
		for i := range privateKey {
			privateKey[i] = 0
		}
	}()
	injectErr := m.configurationManager.InjectX25519Keys(publicKey, privateKey)
	if injectErr != nil {
		return false, fmt.Errorf("failed to inject X25519 key pair: %w", injectErr)
	}
	return true, nil
}

func (m *X25519KeyManager) generateAndStoreKeysInConfiguration() error {
	var private [32]byte
	defer func() {
		for i := range private {
			private[i] = 0
		}
	}()
	if _, err := io.ReadFull(rand.Reader, private[:]); err != nil {
		return fmt.Errorf("failed to generate X25519 private key: %w", err)
	}
	public, err := curve25519.X25519(private[:], curve25519.Basepoint)
	if err != nil {
		return fmt.Errorf("failed to derive X25519 public key: %w", err)
	}
	return m.configurationManager.InjectX25519Keys(public, private[:])
}
