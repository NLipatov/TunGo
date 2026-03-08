package domain

import (
	"path/filepath"
	"strings"
)

func DetectUnitRole(unitBody string) UnitRole {
	for _, line := range strings.Split(unitBody, "\n") {
		normalizedLine := strings.TrimSpace(line)
		if !strings.HasPrefix(normalizedLine, "ExecStart=") {
			continue
		}
		if role := DetectUnitRoleFromExecStart(strings.TrimSpace(strings.TrimPrefix(normalizedLine, "ExecStart="))); role != UnitRoleUnknown {
			return role
		}
	}
	return UnitRoleUnknown
}

func DetectUnitRoleFromExecStart(execStart string) UnitRole {
	if strings.TrimSpace(execStart) == "" || strings.EqualFold(strings.TrimSpace(execStart), "unknown") {
		return UnitRoleUnknown
	}
	cleaned := strings.NewReplacer("{", " ", "}", " ", ";", " ").Replace(execStart)
	fields := strings.Fields(cleaned)
	for i := 0; i < len(fields); i++ {
		current := normalizeExecStartToken(fields[i])
		if !isTungoExecutableToken(current) {
			continue
		}
		for j := i + 1; j < len(fields); j++ {
			next := normalizeExecStartToken(fields[j])
			if next == "" {
				continue
			}
			if isTungoExecutableToken(next) {
				continue
			}
			switch next {
			case "c":
				return UnitRoleClient
			case "s":
				return UnitRoleServer
			default:
				return UnitRoleUnknown
			}
		}
	}
	return UnitRoleUnknown
}

func normalizeExecStartToken(token string) string {
	normalized := strings.TrimSpace(token)
	normalized = strings.Trim(normalized, `"'`)
	normalized = strings.Trim(normalized, ",;")
	if strings.HasPrefix(normalized, "path=") {
		normalized = strings.TrimPrefix(normalized, "path=")
	}
	if strings.HasPrefix(normalized, "argv[]=") {
		normalized = strings.TrimPrefix(normalized, "argv[]=")
	}
	normalized = strings.TrimSpace(normalized)
	normalized = strings.Trim(normalized, `"'`)
	normalized = strings.Trim(normalized, ",;")
	return normalized
}

func isTungoExecutableToken(token string) bool {
	return filepath.Base(strings.TrimPrefix(token, "-")) == "tungo"
}
