package application

import "tungo/settings"

// TunDevice provides a single and trivial API for any supported tun devices
type TunDevice interface {
	Read(data []byte) (int, error)
	Write(data []byte) (int, error)
	Close() error
}

type TunDeviceConfigurator interface {
	// Configure creates a TunDevice
	Configure(s settings.ConnectionSettings) (TunDevice, error)

	// Dispose deletes a TunDevice and cleanup resources
	Dispose(s settings.ConnectionSettings)
}
