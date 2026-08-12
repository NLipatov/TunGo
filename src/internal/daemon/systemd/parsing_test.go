package systemd

import (
	"errors"
	"testing"
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
	if got := ParseUnitFileState([]byte("enabled\n"), nil); got != UnitFileStateEnabled {
		t.Fatalf("expected enabled, got %q", got)
	}
	if got := ParseUnitFileState([]byte("\n"), commandExitError(t, 1)); got != UnitFileStateDisabled {
		t.Fatalf("expected disabled fallback, got %q", got)
	}
	if got := ParseUnitFileState([]byte("\n"), commandExitError(t, 2)); got != UnitFileStateUnknown {
		t.Fatalf("expected unknown, got %q", got)
	}
}

func TestParseUnitActiveState(t *testing.T) {
	if got := ParseUnitActiveState([]byte("active\n"), nil); got != UnitActiveStateActive {
		t.Fatalf("expected active, got %q", got)
	}
	if got := ParseUnitActiveState([]byte("\n"), commandExitError(t, 3)); got != UnitActiveStateInactive {
		t.Fatalf("expected inactive fallback, got %q", got)
	}
	if got := ParseUnitActiveState([]byte("\n"), commandExitError(t, 2)); got != UnitActiveStateUnknown {
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
