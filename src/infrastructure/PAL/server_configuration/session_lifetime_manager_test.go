package server_configuration

import (
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"tungo/infrastructure/settings"
)

type mockServerConfigManager struct {
	injectCalled bool
	ttl          settings.HumanReadableDuration
	cleanup      settings.HumanReadableDuration
	err          error
}

func (m *mockServerConfigManager) Configuration() (*Configuration, error) {
	panic("not implemented")
}

func (m *mockServerConfigManager) IncrementClientCounter() error {
	panic("not implemented")
}

func (m *mockServerConfigManager) InjectEdKeys(_ ed25519.PublicKey, _ ed25519.PrivateKey) error {
	panic("not implemented")
}

func (m *mockServerConfigManager) InjectSessionTtlIntervals(ttl, cleanup settings.HumanReadableDuration) error {
	m.injectCalled = true
	m.ttl = ttl
	m.cleanup = cleanup
	return m.err
}

func TestPrepareSessionLifetime_ValidConfig(t *testing.T) {
	cfg := &Configuration{
		TCPSettings: settings.Settings{
			SessionLifetime: settings.SessionLifetime{
				Ttl:             settings.HumanReadableDuration(10 * time.Minute),
				CleanupInterval: settings.HumanReadableDuration(5 * time.Minute),
			},
		},
		UDPSettings: settings.Settings{
			SessionLifetime: settings.SessionLifetime{
				Ttl:             settings.HumanReadableDuration(10 * time.Minute),
				CleanupInterval: settings.HumanReadableDuration(5 * time.Minute),
			},
		},
	}

	mockManager := &mockServerConfigManager{}

	mgr := NewDefaultSessionLifetimeManager(cfg, mockManager)

	err := mgr.PrepareSessionLifetime()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if mockManager.injectCalled {
		t.Fatalf("expected InjectSessionTtlIntervals NOT to be called")
	}
}

func TestPrepareSessionLifetime_InvalidConfig(t *testing.T) {
	cfg := &Configuration{
		TCPSettings: settings.Settings{
			SessionLifetime: settings.SessionLifetime{
				Ttl:             0,
				CleanupInterval: 0,
			},
		},
		UDPSettings: settings.Settings{
			SessionLifetime: settings.SessionLifetime{
				Ttl:             0,
				CleanupInterval: 0,
			},
		},
	}

	mockManager := &mockServerConfigManager{}

	mgr := NewDefaultSessionLifetimeManager(cfg, mockManager)

	err := mgr.PrepareSessionLifetime()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !mockManager.injectCalled {
		t.Fatalf("expected InjectSessionTtlIntervals to be called")
	}

	if time.Duration(mockManager.ttl) != time.Duration(DefaultSessionTtl) {
		t.Errorf("expected TTL %v, got %v", DefaultSessionTtl, mockManager.ttl)
	}

	if time.Duration(mockManager.cleanup) != time.Duration(DefaultSessionCleanupInterval) {
		t.Errorf("expected CleanupInterval %v, got %v", DefaultSessionCleanupInterval, mockManager.cleanup)
	}
}

func TestPrepareSessionLifetime_InjectReturnsError(t *testing.T) {
	cfg := &Configuration{
		TCPSettings: settings.Settings{
			SessionLifetime: settings.SessionLifetime{
				Ttl:             0,
				CleanupInterval: 0,
			},
		},
		UDPSettings: settings.Settings{
			SessionLifetime: settings.SessionLifetime{
				Ttl:             0,
				CleanupInterval: 0,
			},
		},
	}

	mockManager := &mockServerConfigManager{
		err: errors.New("inject error"),
	}

	mgr := NewDefaultSessionLifetimeManager(cfg, mockManager)

	err := mgr.PrepareSessionLifetime()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	if !mockManager.injectCalled {
		t.Fatalf("expected InjectSessionTtlIntervals to be called")
	}
}

func TestHasValidSessionLifetime(t *testing.T) {
	tests := []struct {
		name   string
		cfg    *Configuration
		expect bool
	}{
		{
			name: "valid values",
			cfg: &Configuration{
				TCPSettings: settings.Settings{
					SessionLifetime: settings.SessionLifetime{
						Ttl:             settings.HumanReadableDuration(1 * time.Second),
						CleanupInterval: settings.HumanReadableDuration(1 * time.Second),
					},
				},
				UDPSettings: settings.Settings{
					SessionLifetime: settings.SessionLifetime{
						Ttl:             settings.HumanReadableDuration(1 * time.Second),
						CleanupInterval: settings.HumanReadableDuration(1 * time.Second),
					},
				},
			},
			expect: true,
		},
		{
			name: "invalid tcp ttl",
			cfg: &Configuration{
				TCPSettings: settings.Settings{
					SessionLifetime: settings.SessionLifetime{
						Ttl:             0,
						CleanupInterval: settings.HumanReadableDuration(1 * time.Second),
					},
				},
				UDPSettings: settings.Settings{
					SessionLifetime: settings.SessionLifetime{
						Ttl:             settings.HumanReadableDuration(1 * time.Second),
						CleanupInterval: settings.HumanReadableDuration(1 * time.Second),
					},
				},
			},
			expect: false,
		},
		{
			name: "invalid udp cleanup interval",
			cfg: &Configuration{
				TCPSettings: settings.Settings{
					SessionLifetime: settings.SessionLifetime{
						Ttl:             settings.HumanReadableDuration(1 * time.Second),
						CleanupInterval: settings.HumanReadableDuration(1 * time.Second),
					},
				},
				UDPSettings: settings.Settings{
					SessionLifetime: settings.SessionLifetime{
						Ttl:             settings.HumanReadableDuration(1 * time.Second),
						CleanupInterval: 0,
					},
				},
			},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewDefaultSessionLifetimeManager(tt.cfg, nil)
			got := mgr.(*DefaultSessionLifetimeManager).hasValidSessionLifetime()
			if got != tt.expect {
				t.Errorf("expected %v, got %v", tt.expect, got)
			}
		})
	}
}
