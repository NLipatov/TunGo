package client

import (
	"os"
	"path/filepath"
)

type DefaultResolver struct {
}

func NewDefaultResolver() DefaultResolver {
	return DefaultResolver{}
}

func (r DefaultResolver) Resolve() (string, error) {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData` // fallback
	}
	return filepath.Join(programData, "TunGo", "client_configuration.json"), nil
}
