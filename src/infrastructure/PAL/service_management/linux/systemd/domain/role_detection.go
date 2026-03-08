package domain

import (
	"path/filepath"
	"strings"
)

func DetectUnitRole(unitBody string) UnitRole {
	for _, line := range strings.Split(unitBody, "\n") {
		if !strings.HasPrefix(line, "ExecStart=") {
			continue
		}
		if role := DetectUnitRoleFromExecStart(strings.TrimSpace(strings.TrimPrefix(line, "ExecStart="))); role != UnitRoleUnknown {
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
		if filepath.Base(current) != "tungo" {
			continue
		}
		for j := i + 1; j < len(fields); j++ {
			switch normalizeExecStartToken(fields[j]) {
			case "c":
				return UnitRoleClient
			case "s":
				return UnitRoleServer
			}
		}
	}
	return UnitRoleUnknown
}

func normalizeExecStartToken(token string) string {
	normalized := strings.TrimSpace(token)
	normalized = strings.Trim(normalized, `"'`)
	normalized = strings.Trim(normalized, ",;")
	normalized = strings.TrimPrefix(normalized, "-")
	if idx := strings.LastIndex(normalized, "="); idx >= 0 {
		normalized = normalized[idx+1:]
	}
	normalized = strings.TrimSpace(normalized)
	normalized = strings.Trim(normalized, `"'`)
	normalized = strings.Trim(normalized, ",;")
	return normalized
}
