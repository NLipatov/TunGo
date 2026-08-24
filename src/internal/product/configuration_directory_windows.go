package product

import (
	"os"
	"path/filepath"
)

// ConfigurationDirectory returns the system directory for TunGo configuration files.
func ConfigurationDirectory() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "TunGo")
}
