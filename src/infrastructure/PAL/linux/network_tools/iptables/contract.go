package iptables

type Contract interface {
	EnableDevMasquerade(devName string) error
	DisableDevMasquerade(devName string) error
	EnableForwardingFromTunToDev(tunName, devName string) error
	DisableForwardingFromTunToDev(tunName, devName string) error
	EnableForwardingFromDevToTun(tunName, devName string) error
	DisableForwardingFromDevToTun(tunName, devName string) error
	ConfigureMssClamping() error
}
