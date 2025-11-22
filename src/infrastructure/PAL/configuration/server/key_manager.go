package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

type KeyManager interface {
	// PrepareKeys guarantees that Ed25519 keys are presented in configuration
	PrepareKeys() error
}

const (
	publicKeyEnvVar  = "ED25519_PUBLIC_KEY"
	privateKeyEnvVar = "ED25519_PRIVATE_KEY"
)

type Ed25519KeyManager struct {
	configurationManager ConfigurationManager
}

func NewEd25519KeyManager(store ConfigurationManager) KeyManager {
	return &Ed25519KeyManager{
		configurationManager: store,
	}
}

func (m *Ed25519KeyManager) PrepareKeys() error {
	if keysAreInConfiguration, err := m.keysAreInConfiguration(); keysAreInConfiguration && err == nil {
		return nil
	}

	if keysAreInEnvVariables, err := m.keysAreInEnvVariables(); keysAreInEnvVariables && err == nil {
		return nil
	}

	return m.generateAndStoreKeysInConfiguration()
}

// keysAreInConfiguration checks if Ed25519 keys are presented in configuration
func (m *Ed25519KeyManager) keysAreInConfiguration() (bool, error) {
	configuration, err := m.configurationManager.Configuration()
	if err == nil {
		// validate public key
		pubKeyPresent := len(configuration.Ed25519PublicKey) > 0
		pubKeyHasValidLength := len(configuration.Ed25519PublicKey) == ed25519.PublicKeySize
		pubKeyIsValid := pubKeyPresent && pubKeyHasValidLength
		// validate private key
		privateKeyPresent := len(configuration.Ed25519PrivateKey) > 0
		privateKeyHasValidLength := len(configuration.Ed25519PrivateKey) == ed25519.PrivateKeySize
		privateKeyIsValid := privateKeyPresent && privateKeyHasValidLength
		return pubKeyIsValid && privateKeyIsValid, nil
	}

	return false, err
}

// keysAreInEnvVariables checks if Ed25519 keys are presented in env variables
func (m *Ed25519KeyManager) keysAreInEnvVariables() (bool, error) {
	// get Ed25519 key pair from env variables
	publicKeyString := os.Getenv(publicKeyEnvVar)
	privateKeyString := os.Getenv(privateKeyEnvVar)
	if publicKeyString == "" || privateKeyString == "" {
		return false, nil // no valid ed25519 key pair in env variables
	}
	// decode Ed25519 key pair from env variables
	publicKey, publicKeyErr := base64.StdEncoding.DecodeString(publicKeyString)
	if publicKeyErr != nil {
		return false, fmt.Errorf("failed to decode public key: %w", publicKeyErr)
	}
	privateKey, privateKeyErr := base64.StdEncoding.DecodeString(privateKeyString)
	if privateKeyErr != nil {
		return false, fmt.Errorf("failed to decode private key: %w", privateKeyErr)
	}
	// inject Ed25519 key pair into configuration
	injectEdKeysErr := m.configurationManager.InjectEdKeys(publicKey, privateKey)
	if injectEdKeysErr != nil {
		return false, fmt.Errorf("failed to inject Ed25519 key pair: %w", injectEdKeysErr)
	}
	return true, nil
}

// generateAndStoreKeysInConfiguration generate new Ed25519 key pair and store it in configuration
func (m *Ed25519KeyManager) generateAndStoreKeysInConfiguration() error {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate Ed25519 key pair: %w", err)
	}
	return m.configurationManager.InjectEdKeys(public, private)
}
