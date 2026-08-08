package tun

import (
	"net/netip"
	"tungo/application/configuration/settings"
)

type ClientManager interface {
	CreateDevice() (Device, error)
	DisposeDevices() error
	SetRouteEndpoint(netip.AddrPort)
}

type ServerManager interface {
	CreateDevice(settings settings.Settings) (Device, error)
	DisposeDevices(settings settings.Settings) error
}
