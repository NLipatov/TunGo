package client

import (
	"os"
	"path/filepath"
)

type pathResolver struct{}

func NewResolver() Resolver {
	return pathResolver{}
}

func (r pathResolver) Resolve() (string, error) {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData` // fallback
	}
	return filepath.Join(programData, "TunGo", "client_configuration.json"), nil
}
