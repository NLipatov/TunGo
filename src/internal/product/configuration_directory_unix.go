//go:build darwin || linux

package product

// ConfigurationDirectory returns the system directory for TunGo configuration files.
func ConfigurationDirectory() string {
	return "/etc/tungo"
}
