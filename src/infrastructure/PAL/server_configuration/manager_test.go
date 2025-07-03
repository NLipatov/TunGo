package server_configuration

import (
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"tungo/infrastructure/settings"
)

// managerTestMockErrorResolver returns an error from resolve().
type managerTestMockErrorResolver struct{}

func (r managerTestMockErrorResolver) Resolve() (string, error) {
	return "", errors.New("resolve error")
}

// managerTestMockBadPathResolver returns an invalid path to simulate write error.
type managerTestMockBadPathResolver struct{}

func (r managerTestMockBadPathResolver) Resolve() (string, error) {
	// invalid path with null byte
	return string([]byte{0}), nil
}

// managerTestValidResolver returns a valid file path.
type managerTestValidResolver struct {
	path string
}

func (r managerTestValidResolver) Resolve() (string, error) {
	return r.path, nil
}

func createTestConfigPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "conf.json")
}

func TestManagerConfigurationWriteDefaultError(t *testing.T) {
	manager, managerErr := NewManager(managerTestMockBadPathResolver{})
	if managerErr != nil {
		t.Error(managerErr)
	}

	_, err := manager.Configuration()
	if err == nil {
		t.Fatal("expected error from Configuration() due to write default configuration failure, got nil")
	}
	if !strings.Contains(err.Error(), "could not write default configuration") {
		t.Errorf("expected error to mention 'could not write default configuration', got %v", err)
	}
}

func TestManagerConfigurationReadSuccess(t *testing.T) {
	path := createTestConfigPath(t)
	manager, managerErr := NewManager(managerTestValidResolver{path: path})
	if managerErr != nil {
		t.Error(managerErr)
	}

	// Ensure file does not exist initially.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file not to exist, but it does")
	}

	conf, err := manager.Configuration()
	if err != nil {
		t.Fatalf("Configuration() error: %v", err)
	}

	// At this point, the default configuration should have been written.
	defaultConf := NewDefaultConfiguration()
	if !reflect.DeepEqual(conf, defaultConf) {
		t.Errorf("expected default configuration %v, got %v", defaultConf, conf)
	}

	// File should now exist.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist after default configuration creation, got error: %v", err)
	}
}

func TestIncrementClientCounterSuccess(t *testing.T) {
	path := createTestConfigPath(t)
	manager, managerErr := NewManager(managerTestValidResolver{path: path})
	if managerErr != nil {
		t.Error(managerErr)
	}

	// Create initial configuration.
	conf, err := manager.Configuration()
	if err != nil {
		t.Fatalf("Configuration() error: %v", err)
	}
	initialCounter := conf.ClientCounter

	// This call covers:
	// configuration.ClientCounter += 1
	// w := newWriter(c.resolver)
	// return w.Write(*configuration)
	if err := manager.IncrementClientCounter(); err != nil {
		t.Fatalf("IncrementClientCounter() error: %v", err)
	}

	updatedConf, err := manager.Configuration()
	if err != nil {
		t.Fatalf("Configuration() error after increment: %v", err)
	}

	if updatedConf.ClientCounter != initialCounter+1 {
		t.Errorf("expected ClientCounter %d, got %d", initialCounter+1, updatedConf.ClientCounter)
	}
}

func TestInjectEdKeysSuccess(t *testing.T) {
	path := createTestConfigPath(t)
	manager, managerErr := NewManager(managerTestValidResolver{path: path})
	if managerErr != nil {
		t.Error(managerErr)
	}

	// Initialize configuration.
	if _, err := manager.Configuration(); err != nil {
		t.Fatalf("Configuration() error: %v", err)
	}

	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate keys: %v", err)
	}

	// This call covers:
	// configuration.Ed25519PublicKey = public
	// configuration.Ed25519PrivateKey = private
	// w := newWriter(c.resolver)
	// return w.Write(*configuration)
	if err := manager.InjectEdKeys(public, private); err != nil {
		t.Fatalf("InjectEdKeys() error: %v", err)
	}

	conf, err := manager.Configuration()
	if err != nil {
		t.Fatalf("Configuration() error after key injection: %v", err)
	}

	if !reflect.DeepEqual(conf.Ed25519PublicKey, public) {
		t.Errorf("expected public key %v, got %v", public, conf.Ed25519PublicKey)
	}
	if !reflect.DeepEqual(conf.Ed25519PrivateKey, private) {
		t.Errorf("expected private key %v, got %v", private, conf.Ed25519PrivateKey)
	}
}

func TestIncrementClientCounterConfigError(t *testing.T) {
	_, err := NewManager(managerTestMockErrorResolver{})
	if err == nil {
		t.Fatal("expected error from NewManager() due to configuration path resolve failure, got nil")
	}

	if !strings.Contains(err.Error(), "failed to resolve server configuration path") {
		t.Errorf("expected error to mention 'failed to resolve server configuration path', got %v", err)
	}
}

func TestInjectEdKeysConfigError(t *testing.T) {
	_, err := NewManager(managerTestMockErrorResolver{})
	if err == nil {
		t.Fatal("expected error from InjectEdKeys() due to configuration failure, got nil")
	}
	if !strings.Contains(err.Error(), "failed to resolve server configuration path") {
		t.Errorf("expected error to mention 'failed to resolve server configuration path', got %v", err)
	}
}
func TestInjectSessionTtlIntervals_Success(t *testing.T) {
	path := createTestConfigPath(t)
	manager, managerErr := NewManager(managerTestValidResolver{path: path})
	if managerErr != nil {
		t.Error(managerErr)
	}

	_, err := manager.Configuration()
	if err != nil {
		t.Fatalf("Configuration() error: %v", err)
	}

	newTTL := settings.HumanReadableDuration(30 * time.Minute)
	newCleanup := settings.HumanReadableDuration(15 * time.Minute)

	err = manager.InjectSessionTtlIntervals(newTTL, newCleanup)
	if err != nil {
		t.Fatalf("InjectSessionTtlIntervals() error: %v", err)
	}

	conf, err := manager.Configuration()
	if err != nil {
		t.Fatalf("Configuration() error after inject: %v", err)
	}

	if conf.TCPSettings.SessionLifetime.Ttl != newTTL {
		t.Errorf("expected TCP TTL %v, got %v", newTTL, conf.TCPSettings.SessionLifetime.Ttl)
	}
	if conf.TCPSettings.SessionLifetime.CleanupInterval != newCleanup {
		t.Errorf("expected TCP CleanupInterval %v, got %v", newCleanup, conf.TCPSettings.SessionLifetime.CleanupInterval)
	}
	if conf.UDPSettings.SessionLifetime.Ttl != newTTL {
		t.Errorf("expected UDP TTL %v, got %v", newTTL, conf.UDPSettings.SessionLifetime.Ttl)
	}
	if conf.UDPSettings.SessionLifetime.CleanupInterval != newCleanup {
		t.Errorf("expected UDP CleanupInterval %v, got %v", newCleanup, conf.UDPSettings.SessionLifetime.CleanupInterval)
	}
}

func TestInjectSessionTtlIntervals_ConfigurationError(t *testing.T) {
	_, err := NewManager(managerTestMockErrorResolver{})
	if err == nil {
		t.Fatal("expected error due to configuration resolution failure, got nil")
	}
	if !strings.Contains(err.Error(), "failed to resolve server configuration path") {
		t.Errorf("expected error mentioning 'failed to resolve server configuration path', got %v", err)
	}
}

func TestInjectSessionTtlIntervals_WriteError(t *testing.T) {
	manager, managerErr := NewManager(managerTestMockBadPathResolver{})
	if managerErr != nil {
		t.Error(managerErr)
	}

	err := manager.InjectSessionTtlIntervals(
		settings.HumanReadableDuration(10*time.Minute),
		settings.HumanReadableDuration(5*time.Minute),
	)
	if err == nil {
		t.Fatal("expected error due to write failure, got nil")
	}
	if !strings.Contains(err.Error(), "could not write default configuration") {
		t.Errorf("expected error mentioning 'could not write default configuration', got %v", err)
	}
}
