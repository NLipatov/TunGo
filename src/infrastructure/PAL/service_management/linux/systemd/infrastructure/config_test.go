package infrastructure

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.RuntimeDir != "/run/systemd/system" ||
		cfg.UnitPath != "/etc/systemd/system/tungo.service" ||
		cfg.UnitName != "tungo.service" ||
		cfg.BinaryPath != "/usr/local/bin/tungo" {
		t.Fatalf("unexpected default config: %+v", cfg)
	}
}
