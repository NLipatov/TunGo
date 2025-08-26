package nftables

import (
	"sync"
	"tungo/application"
)

// Ensure decorator implements the interface.
var _ application.Netfilter = (*SynchronizedNetfilter)(nil)

type SynchronizedNetfilter struct {
	netfilter application.Netfilter
	mu        sync.Mutex
}

func NewSynchronizedNetfilter(netfilter application.Netfilter) *SynchronizedNetfilter {
	return &SynchronizedNetfilter{
		netfilter: netfilter,
	}
}

func (d *SynchronizedNetfilter) EnableDevMasquerade(devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.netfilter.EnableDevMasquerade(devName)
}
func (d *SynchronizedNetfilter) DisableDevMasquerade(devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.netfilter.DisableDevMasquerade(devName)
}
func (d *SynchronizedNetfilter) EnableForwardingFromTunToDev(tunName, devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.netfilter.EnableForwardingFromTunToDev(tunName, devName)
}
func (d *SynchronizedNetfilter) DisableForwardingFromTunToDev(tunName, devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.netfilter.DisableForwardingFromTunToDev(tunName, devName)
}
func (d *SynchronizedNetfilter) EnableForwardingFromDevToTun(tunName, devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.netfilter.EnableForwardingFromDevToTun(tunName, devName)
}
func (d *SynchronizedNetfilter) DisableForwardingFromDevToTun(tunName, devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.netfilter.DisableForwardingFromDevToTun(tunName, devName)
}
func (d *SynchronizedNetfilter) ConfigureMssClamping() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.netfilter.ConfigureMssClamping()
}
