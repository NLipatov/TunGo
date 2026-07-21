//go:build linux

package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"tungo/application/configuration"
)

const defaultConfigTestLock = "/tmp/tungo-default-configuration-test.lock"

func acquireDefaultConfigTestLock(t *testing.T) {
	t.Helper()
	lock, err := os.OpenFile(defaultConfigTestLock, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatalf("open default configuration test lock: %v", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		_ = lock.Close()
		t.Fatalf("acquire default configuration test lock: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
	})
}

func TestNewDefaultRuntimesOnCleanSystem(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("default server configuration requires root access")
	}

	acquireDefaultConfigTestLock(t)
	directory := filepath.Join(string(filepath.Separator), "etc", "tungo")
	if _, err := os.Stat(directory); err == nil {
		t.Skip("preserving existing /etc/tungo")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat default configuration directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(filepath.Join(directory, "client_configuration.json.1"))
		_ = os.Remove(filepath.Join(directory, "client_configuration.json"))
		_ = os.Remove(filepath.Join(directory, "server_configuration.json"))
		_ = os.Remove(filepath.Join(directory, "crash.log"))
		_ = os.Remove(directory)
	})
	t.Setenv("ServerIP", "127.0.0.1")

	clientRuntime, err := New(ModeClient)
	if err == nil || !strings.Contains(err.Error(), "client configuration") {
		t.Fatalf("New(ModeClient) = %T, %v; want configuration error", clientRuntime, err)
	}

	serverRuntime, err := New(ModeServer)
	if err != nil {
		t.Fatalf("New(ModeServer) error = %v", err)
	}
	if serverRuntime == nil {
		t.Fatal("New(ModeServer) returned nil runtime")
	}
	if serverRuntime.Ready() {
		t.Fatal("new server runtime reported ready before Run")
	}

	control := configuration.NewDefaultServerControl()
	generated, err := control.GenerateClientConfiguration()
	if err != nil {
		t.Fatalf("GenerateClientConfiguration() error = %v", err)
	}
	serialized, err := os.ReadFile(generated.Path)
	if err != nil {
		t.Fatalf("read generated client configuration: %v", err)
	}
	clientPath := filepath.Join(directory, "client_configuration.json")
	if err := os.WriteFile(clientPath, serialized, 0600); err != nil {
		t.Fatalf("write default client configuration: %v", err)
	}

	clientRuntime, err = New(ModeClient)
	if err != nil {
		t.Fatalf("New(ModeClient) error = %v", err)
	}
	if clientRuntime == nil {
		t.Fatal("New(ModeClient) returned nil runtime")
	}
	if clientRuntime.Ready() {
		t.Fatal("new client runtime reported ready before Run")
	}
}
