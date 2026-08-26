package configpath

import (
	"os"
	"path/filepath"
)

// Directory returns the system directory for TunGo configuration files.
func Directory() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "TunGo")
}
