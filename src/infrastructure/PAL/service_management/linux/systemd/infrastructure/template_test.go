package infrastructure

import (
	"strings"
	"testing"
)

func TestUnitFileContent(t *testing.T) {
	content := UnitFileContent("/usr/local/bin/tungo", "c")
	if !strings.Contains(content, "ExecStart=/usr/local/bin/tungo c") {
		t.Fatalf("expected exec start line, got %q", content)
	}
	if !strings.Contains(content, "WantedBy=multi-user.target") {
		t.Fatalf("expected install section, got %q", content)
	}
}
