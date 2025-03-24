package client_configuration

import (
	"os"
	"path/filepath"
)

type clientResolver struct {
}

func newClientResolver() clientResolver {
	return clientResolver{}
}

func (r clientResolver) resolve() (string, error) {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData` // fallback
	}
	return filepath.Join(programData, "TunGo", "client_configuration.json"), nil
}
