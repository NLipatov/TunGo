package iptables

type Contract interface {
	EnableDevMasquerade(devName, sourceCIDR string) error
	DisableDevMasquerade(devName, sourceCIDR string) error
	EnableForwardingFromTunToDev(tunName, devName string) error
	DisableForwardingFromTunToDev(tunName, devName string) error
	EnableForwardingFromDevToTun(tunName, devName string) error
	DisableForwardingFromDevToTun(tunName, devName string) error
	EnableForwardingTunToTun(tunName string) error
	DisableForwardingTunToTun(tunName string) error

	Enable6DevMasquerade(devName, sourceCIDR string) error
	Disable6DevMasquerade(devName, sourceCIDR string) error
	Enable6ForwardingFromTunToDev(tunName, devName string) error
	Disable6ForwardingFromTunToDev(tunName, devName string) error
	Enable6ForwardingFromDevToTun(tunName, devName string) error
	Disable6ForwardingFromDevToTun(tunName, devName string) error
	Enable6ForwardingTunToTun(tunName string) error
	Disable6ForwardingTunToTun(tunName string) error
}
