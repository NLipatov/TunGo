package systemd

import (
	"strings"
	"testing"
)

func TestUnitFileContent(t *testing.T) {
	content := UnitFileContent("/usr/local/bin/tungo", []string{"c"})
	if !strings.Contains(content, `ExecStart="/usr/local/bin/tungo" c`) {
		t.Fatalf("expected exec start line, got %q", content)
	}
	if !strings.Contains(content, "WantedBy=multi-user.target") {
		t.Fatalf("expected install section, got %q", content)
	}
}

func TestUnitFileContent_QuotesBinaryPathWithSpaces(t *testing.T) {
	content := UnitFileContent("/opt/TunGo VPN/tungo", []string{"s"})
	if !strings.Contains(content, `ExecStart="/opt/TunGo VPN/tungo" s`) {
		t.Fatalf("expected quoted binary path, got %q", content)
	}
}
