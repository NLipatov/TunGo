package systemd

import (
	"errors"
	"os/exec"
	"strings"
)

func IsSystemdNotActiveError(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	code := exitErr.ExitCode()
	return code == 3 || code == 4
}

func IsSystemdDisabledError(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	code := exitErr.ExitCode()
	return code == 1 || code == 3 || code == 4
}

func ParseUnitFileState(output []byte, err error) UnitFileState {
	state := UnitFileState(NormalizeSystemdValue(string(output)))
	if state == UnitFileStateUnknown && err != nil && IsSystemdDisabledError(err) {
		return UnitFileStateDisabled
	}
	return state
}

func ParseUnitActiveState(output []byte, err error) UnitActiveState {
	state := UnitActiveState(NormalizeSystemdValue(string(output)))
	if state == UnitActiveStateUnknown && err != nil && IsSystemdNotActiveError(err) {
		return UnitActiveStateInactive
	}
	return state
}

func ParseSystemdShowProperties(output []byte) map[string]string {
	properties := make(map[string]string, 4)
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		properties[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return properties
}

func NormalizeSystemdValue(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "unknown"
	}
	return normalized
}

func NormalizeSystemdRawValue(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "unknown"
	}
	return normalized
}
