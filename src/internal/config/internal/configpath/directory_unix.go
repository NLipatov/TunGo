//go:build darwin || linux

package configpath

// Directory returns the system directory for TunGo configuration files.
func Directory() string {
	return "/etc/tungo"
}
