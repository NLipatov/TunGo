package application

type TunManager interface {
	CreateTunDevice() (TunDevice, error)
	DisposeTunDevices() error
}
