package domain

import "testing"

func TestActiveStateBlocksRuntimeStart(t *testing.T) {
	cases := []struct {
		state UnitActiveState
		want  bool
	}{
		{UnitActiveStateActive, true},
		{UnitActiveStateReloading, true},
		{UnitActiveStateActivating, true},
		{UnitActiveStateDeactivating, true},
		{UnitActiveStateInactive, false},
		{UnitActiveStateFailed, false},
		{UnitActiveStateUnknown, false},
	}
	for _, tc := range cases {
		if got := ActiveStateBlocksRuntimeStart(tc.state); got != tc.want {
			t.Fatalf("state=%q: got %v want %v", tc.state, got, tc.want)
		}
	}
}

func TestActiveStateIndicatesRunning(t *testing.T) {
	cases := []struct {
		state UnitActiveState
		want  bool
	}{
		{UnitActiveStateActive, true},
		{UnitActiveStateReloading, true},
		{UnitActiveStateActivating, false},
		{UnitActiveStateInactive, false},
		{UnitActiveStateUnknown, false},
	}
	for _, tc := range cases {
		if got := ActiveStateIndicatesRunning(tc.state); got != tc.want {
			t.Fatalf("state=%q: got %v want %v", tc.state, got, tc.want)
		}
	}
}

func TestIsInstallerManagedFragmentPath(t *testing.T) {
	if IsInstallerManagedFragmentPath("", "/etc/systemd/system/tungo.service") {
		t.Fatal("expected empty fragment path to be unmanaged")
	}
	if IsInstallerManagedFragmentPath("unknown", "/etc/systemd/system/tungo.service") {
		t.Fatal("expected unknown fragment path to be unmanaged")
	}
	if !IsInstallerManagedFragmentPath(" /etc/systemd/system/tungo.service ", "/etc/systemd/system/tungo.service") {
		t.Fatal("expected matching /etc path to be managed")
	}
	if IsInstallerManagedFragmentPath("/usr/lib/systemd/system/tungo.service", "/etc/systemd/system/tungo.service") {
		t.Fatal("expected non-/etc fragment to be unmanaged")
	}
}
