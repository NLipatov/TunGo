//go:build darwin

package scutil

// Contract is a thin wrapper over macOS scutil utility.
type Contract interface {
	AddScopedDNSResolver(
		interfaceName string,
		nameservers []string,
		domains []string,
	) error

	RemoveScopedDNSResolver(interfaceName string) error
}

const (
	scutilBinary = "scutil"
)
