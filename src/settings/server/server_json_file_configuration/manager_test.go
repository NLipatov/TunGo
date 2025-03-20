package server_json_file_configuration

import (
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"
)

// managerTestMockErrorResolver returns an error from resolve().
type managerTestMockErrorResolver struct{}

func (r managerTestMockErrorResolver) resolve() (string, error) {
	return "", errors.New("resolve error")
}

// managerTestMockBadPathResolver returns an invalid path to simulate write error.
type managerTestMockBadPathResolver struct{}

func (r managerTestMockBadPathResolver) resolve() (string, error) {
	// invalid path with null byte
	return string([]byte{0}), nil
}

func TestManagerConfigurationResolverError(t *testing.T) {
	manager := NewManager()
	manager.resolver = managerTestMockErrorResolver{}

	_, err := manager.Configuration()
	if err == nil {
		t.Fatal("expected error from Configuration() due to resolver error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read configuration") {
		t.Errorf("expected error to mention 'failed to read configuration', got %v", err)
	}
}

func TestManagerConfigurationWriteDefaultError(t *testing.T) {
	manager := NewManager()
	manager.resolver = managerTestMockBadPathResolver{}

	_, err := manager.Configuration()
	if err == nil {
		t.Fatal("expected error from Configuration() due to write default configuration failure, got nil")
	}
	if !strings.Contains(err.Error(), "could not write default configuration") {
		t.Errorf("expected error to mention 'could not write default configuration', got %v", err)
	}
}

func TestIncrementClientCounterConfigError(t *testing.T) {
	manager := NewManager()
	manager.resolver = managerTestMockErrorResolver{}

	err := manager.IncrementClientCounter()
	if err == nil {
		t.Fatal("expected error from IncrementClientCounter() due to Configuration() failure, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read configuration") {
		t.Errorf("expected error to mention 'failed to read configuration', got %v", err)
	}
}

func TestInjectEdKeysConfigError(t *testing.T) {
	manager := NewManager()
	manager.resolver = managerTestMockErrorResolver{}

	public, private, genErr := ed25519.GenerateKey(nil)
	if genErr != nil {
		t.Fatalf("failed to generate keys: %v", genErr)
	}

	err := manager.InjectEdKeys(public, private)
	if err == nil {
		t.Fatal("expected error from InjectEdKeys() due to Configuration() failure, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read configuration") {
		t.Errorf("expected error to mention 'failed to read configuration', got %v", err)
	}
}
