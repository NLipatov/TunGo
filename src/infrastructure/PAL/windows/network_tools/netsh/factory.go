package netsh

import (
	"fmt"
	"net"
	"strings"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/settings"
)

type Factory struct {
	connectionSettings settings.Settings
	commander          PAL.Commander
}

func NewFactory(connectionSettings settings.Settings, commander PAL.Commander) *Factory {
	return &Factory{
		connectionSettings: connectionSettings,
		commander:          commander}
}

func (f *Factory) CreateNetsh() (Contract, error) {
	addr := strings.TrimSpace(f.connectionSettings.InterfaceAddress)
	if i := strings.IndexByte(addr, '%'); i >= 0 {
		addr = addr[:i]
	}
	ip := net.ParseIP(addr)
	if ip == nil {
		return nil, fmt.Errorf("invalid InterfaceAddress: %q", f.connectionSettings.InterfaceAddress)
	}
	if ip.IsUnspecified() {
		return nil, fmt.Errorf("unspecified InterfaceAddress not allowed: %q", f.connectionSettings.InterfaceAddress)
	}
	if ip.To4() != nil {
		return NewV4Wrapper(f.commander), nil
	}
	return NewV6Wrapper(f.commander), nil
}
