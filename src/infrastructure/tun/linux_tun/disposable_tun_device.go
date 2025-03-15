package linux_tun

import (
	"errors"
	"tungo/application"
	"tungo/infrastructure/network/ip"
	"tungo/settings"
)

type DisposableTunDevice struct {
	device   application.TunDevice
	settings settings.ConnectionSettings
}

func NewDisposableTunDevice(device application.TunDevice, settings settings.ConnectionSettings) application.TunDevice {
	return &DisposableTunDevice{
		device:   device,
		settings: settings,
	}
}

func (d *DisposableTunDevice) Read(buffer []byte) (int, error) {
	return d.device.Read(buffer)
}

func (d *DisposableTunDevice) Write(data []byte) (int, error) {
	return d.device.Write(data)
}

func (d *DisposableTunDevice) Close() error {
	_ = d.device.Close()
	return d.Dispose()
}

func (d *DisposableTunDevice) Dispose() error {
	// Delete route to server
	deleteRouteErr := ip.RouteDel(d.settings.ConnectionIP)

	// Delete the TUN interface
	_, deleteTunErr := ip.LinkDel(d.settings.InterfaceName)

	if deleteRouteErr != nil || deleteTunErr != nil {
		return errors.Join(deleteRouteErr, deleteTunErr)
	}

	return nil
}
