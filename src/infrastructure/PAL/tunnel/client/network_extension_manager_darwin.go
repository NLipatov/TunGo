//go:build darwin

package client

import (
	"fmt"
	"net/netip"
	"sync"

	"golang.org/x/sys/unix"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/network/darwin/utun"
)

var networkExtensionDescriptor struct {
	sync.Mutex
	fd         int
	generation uint64
	active     bool
}

// RegisterNetworkExtensionFileDescriptor makes a system-owned UTUN descriptor
// available to the normal Darwin client TUN manager. The returned function
// removes only this registration and never closes the descriptor owned by
// NetworkExtension.
func RegisterNetworkExtensionFileDescriptor(fd int) (release func(), err error) {
	if fd < 0 {
		return nil, fmt.Errorf("invalid NetworkExtension tunnel file descriptor %d", fd)
	}
	probe, err := unix.Dup(fd)
	if err != nil {
		return nil, fmt.Errorf("validate NetworkExtension tunnel file descriptor: %w", err)
	}
	_ = unix.Close(probe)

	networkExtensionDescriptor.Lock()
	defer networkExtensionDescriptor.Unlock()
	if networkExtensionDescriptor.active {
		return nil, fmt.Errorf("a NetworkExtension tunnel descriptor is already registered")
	}
	networkExtensionDescriptor.generation++
	generation := networkExtensionDescriptor.generation
	networkExtensionDescriptor.fd = fd
	networkExtensionDescriptor.active = true

	var once sync.Once
	return func() {
		once.Do(func() {
			networkExtensionDescriptor.Lock()
			defer networkExtensionDescriptor.Unlock()
			if networkExtensionDescriptor.active && networkExtensionDescriptor.generation == generation {
				networkExtensionDescriptor.fd = -1
				networkExtensionDescriptor.active = false
			}
		})
	}, nil
}

func newNetworkExtensionTunManager() (tun.ClientManager, bool) {
	networkExtensionDescriptor.Lock()
	defer networkExtensionDescriptor.Unlock()
	if !networkExtensionDescriptor.active {
		return nil, false
	}
	return &networkExtensionTunManager{fd: networkExtensionDescriptor.fd}, true
}

type networkExtensionTunManager struct {
	fd int
}

func (m *networkExtensionTunManager) CreateDevice() (tun.Device, error) {
	device, err := newFileDescriptorUTUN(m.fd)
	if err != nil {
		return nil, err
	}
	return utun.NewDarwinTunDevice(device), nil
}

// NetworkExtension owns both the source descriptor and interface lifecycle.
func (m *networkExtensionTunManager) DisposeDevices() error { return nil }

// NetworkExtension settings, rather than route(8), keep provider traffic out
// of the tunnel and install the virtual interface routes.
func (m *networkExtensionTunManager) SetRouteEndpoint(netip.AddrPort) {}
