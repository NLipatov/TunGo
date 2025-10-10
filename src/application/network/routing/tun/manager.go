package tun

import (
	"tungo/infrastructure/settings"
)

type ClientManager interface {
	CreateDevice() (Device, error)
	DisposeDevices() error
}

type ServerManager interface {
	CreateDevice(settings settings.Settings) (Device, error)
	DisposeDevices(settings settings.Settings) error
}
