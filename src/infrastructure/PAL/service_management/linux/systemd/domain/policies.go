package domain

import (
	"path/filepath"
	"strings"
)

// ActiveStateBlocksRuntimeStart encodes runtime safety invariant.
func ActiveStateBlocksRuntimeStart(state UnitActiveState) bool {
	switch state {
	case UnitActiveStateActive, UnitActiveStateReloading, UnitActiveStateActivating, UnitActiveStateDeactivating:
		return true
	default:
		return false
	}
}

func ActiveStateIndicatesRunning(state UnitActiveState) bool {
	switch state {
	case UnitActiveStateActive, UnitActiveStateReloading:
		return true
	default:
		return false
	}
}

func IsInstallerManagedFragmentPath(fragmentPath string, managedPath string) bool {
	normalized := strings.TrimSpace(fragmentPath)
	if normalized == "" || strings.EqualFold(normalized, "unknown") {
		return false
	}
	return filepath.Clean(normalized) == filepath.Clean(managedPath)
}
