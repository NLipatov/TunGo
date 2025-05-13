package iptables

type Contract interface {
	EnableMasquerade(devName string) error
	DisableMasquerade(devName string) error
	AcceptForwardFromTunToDev(tunName, devName string) error
	DropForwardFromTunToDev(tunName, devName string) error
	AcceptForwardFromDevToTun(tunName, devName string) error
	DropForwardFromDevToTun(tunName, devName string) error
	ConfigureMssClamping() error
}
