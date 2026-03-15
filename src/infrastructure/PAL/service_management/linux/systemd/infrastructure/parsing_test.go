package infrastructure

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"tungo/infrastructure/PAL/service_management/linux/systemd/domain"
)

func TestIsSystemdNotActiveError(t *testing.T) {
	if IsSystemdNotActiveError(errors.New("boom")) {
		t.Fatal("expected false for non-exit error")
	}
	if !IsSystemdNotActiveError(commandExitError(t, 3)) {
		t.Fatal("expected true for exit code 3")
	}
	if !IsSystemdNotActiveError(commandExitError(t, 4)) {
		t.Fatal("expected true for exit code 4")
	}
	if IsSystemdNotActiveError(commandExitError(t, 1)) {
		t.Fatal("expected false for exit code 1")
	}
}

func TestIsSystemdDisabledError(t *testing.T) {
	if IsSystemdDisabledError(errors.New("boom")) {
		t.Fatal("expected false for non-exit error")
	}
	if !IsSystemdDisabledError(commandExitError(t, 1)) {
		t.Fatal("expected true for exit code 1")
	}
	if !IsSystemdDisabledError(commandExitError(t, 3)) {
		t.Fatal("expected true for exit code 3")
	}
	if !IsSystemdDisabledError(commandExitError(t, 4)) {
		t.Fatal("expected true for exit code 4")
	}
	if IsSystemdDisabledError(commandExitError(t, 2)) {
		t.Fatal("expected false for exit code 2")
	}
}

func TestParseUnitFileState(t *testing.T) {
	if got := ParseUnitFileState([]byte("enabled\n"), nil); got != domain.UnitFileStateEnabled {
		t.Fatalf("expected enabled, got %q", got)
	}
	if got := ParseUnitFileState([]byte("\n"), commandExitError(t, 1)); got != domain.UnitFileStateDisabled {
		t.Fatalf("expected disabled fallback, got %q", got)
	}
	if got := ParseUnitFileState([]byte("\n"), commandExitError(t, 2)); got != domain.UnitFileStateUnknown {
		t.Fatalf("expected unknown, got %q", got)
	}
}

func TestParseUnitActiveState(t *testing.T) {
	if got := ParseUnitActiveState([]byte("active\n"), nil); got != domain.UnitActiveStateActive {
		t.Fatalf("expected active, got %q", got)
	}
	if got := ParseUnitActiveState([]byte("\n"), commandExitError(t, 3)); got != domain.UnitActiveStateInactive {
		t.Fatalf("expected inactive fallback, got %q", got)
	}
	if got := ParseUnitActiveState([]byte("\n"), commandExitError(t, 2)); got != domain.UnitActiveStateUnknown {
		t.Fatalf("expected unknown, got %q", got)
	}
}

func TestParseSystemdShowProperties(t *testing.T) {
	props := ParseSystemdShowProperties([]byte("LoadState=loaded\nInvalidLine\nKey=Value=Tail\n\n"))
	if props["LoadState"] != "loaded" {
		t.Fatalf("expected LoadState=loaded, got %q", props["LoadState"])
	}
	if props["Key"] != "Value=Tail" {
		t.Fatalf("expected Key=Value=Tail, got %q", props["Key"])
	}
	if _, ok := props["InvalidLine"]; ok {
		t.Fatal("did not expect invalid line to become property")
	}
}

func TestNormalizeSystemdValue(t *testing.T) {
	if got := NormalizeSystemdValue("  ACTIVE \n"); got != "active" {
		t.Fatalf("expected active, got %q", got)
	}
	if got := NormalizeSystemdValue("   \n"); got != "unknown" {
		t.Fatalf("expected unknown, got %q", got)
	}
}

func TestNormalizeSystemdRawValue(t *testing.T) {
	if got := NormalizeSystemdRawValue("  A B  "); got != "A B" {
		t.Fatalf("expected trimmed value, got %q", got)
	}
	if got := NormalizeSystemdRawValue("\n"); got != "unknown" {
		t.Fatalf("expected unknown, got %q", got)
	}
}

func commandExitError(t *testing.T, code int) error {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcessExit")
	cmd.Env = append(
		os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		fmt.Sprintf("GO_HELPER_EXIT_CODE=%d", code),
	)
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit for code %d", code)
	}
	return err
}

func TestHelperProcessExit(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	code, err := strconv.Atoi(os.Getenv("GO_HELPER_EXIT_CODE"))
	if err != nil {
		os.Exit(2)
	}
	os.Exit(code)
}
