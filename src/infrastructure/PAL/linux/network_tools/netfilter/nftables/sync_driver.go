package nftables

import (
	"sync"
	"tungo/application"
)

// Ensure decorator implements the interface.
var _ application.Netfilter = (*SyncDriver)(nil)

type SyncDriver struct {
	netfilter application.Netfilter
	mu        sync.Mutex
}

func NewSyncDriver(netfilter application.Netfilter) *SyncDriver {
	return &SyncDriver{
		netfilter: netfilter,
	}
}

func (d *SyncDriver) EnableDevMasquerade(devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.netfilter.EnableDevMasquerade(devName)
}
func (d *SyncDriver) DisableDevMasquerade(devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.netfilter.DisableDevMasquerade(devName)
}
func (d *SyncDriver) EnableForwardingFromTunToDev(tunName, devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.netfilter.EnableForwardingFromTunToDev(tunName, devName)
}
func (d *SyncDriver) DisableForwardingFromTunToDev(tunName, devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.netfilter.DisableForwardingFromTunToDev(tunName, devName)
}
func (d *SyncDriver) EnableForwardingFromDevToTun(tunName, devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.netfilter.EnableForwardingFromDevToTun(tunName, devName)
}
func (d *SyncDriver) DisableForwardingFromDevToTun(tunName, devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.netfilter.DisableForwardingFromDevToTun(tunName, devName)
}
func (d *SyncDriver) ConfigureMssClamping(devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.netfilter.ConfigureMssClamping(devName)
}
