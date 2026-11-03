package systemd

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.RuntimeDir != "/run/systemd/system" ||
		cfg.UnitDir != "/etc/systemd/system" ||
		cfg.BinaryPath != "/usr/local/bin/tungo" {
		t.Fatalf("unexpected default config: %+v", cfg)
	}
}
