package server

import (
	"fmt"
	"log"
	"os"
	"tungo/infrastructure/PAL/network/linux/ioctl"
	"tungo/infrastructure/PAL/network/linux/ip"
	"tungo/infrastructure/settings"
)

type tunDeviceManager struct {
	ip    ip.Contract
	ioctl ioctl.Contract
}

func (d tunDeviceManager) create(s settings.Settings, ipv4, ipv6 bool) (tunFile *os.File, err error) {
	created := false
	defer func() {
		if err != nil && created {
			if delErr := d.ip.LinkDelete(s.TunName); delErr != nil {
				log.Printf("failed to rollback TUN %s after create error: %v", s.TunName, delErr)
			}
		}
	}()

	// delete previous tun if any exist
	_ = d.ip.LinkDelete(s.TunName)

	if err = d.ip.TunTapAddDevTun(s.TunName); err != nil {
		return nil, fmt.Errorf("could not create tuntap dev: %s", err)
	}
	created = true

	if err = d.ip.LinkSetDevUp(s.TunName); err != nil {
		return nil, fmt.Errorf("could not set tuntap dev up: %s", err)
	}

	if err = d.ip.LinkSetDevMTU(s.TunName, s.MTU); err != nil {
		return nil, fmt.Errorf("could not set mtu on tuntap dev: %s", err)
	}

	hasAddress := false
	if ipv4 {
		cidr4, cidr4Err := s.IPv4CIDR()
		if cidr4Err != nil {
			return nil, fmt.Errorf("could not derive server IPv4 CIDR: %s", cidr4Err)
		}
		if err = d.ip.AddrAddDev(s.TunName, cidr4); err != nil {
			return nil, fmt.Errorf("failed to convert server ip to CIDR format: %s", err)
		}
		hasAddress = true
	}

	if ipv6 {
		cidr6, cidr6Err := s.IPv6CIDR()
		if cidr6Err != nil {
			return nil, fmt.Errorf("could not derive server IPv6 CIDR: %s", cidr6Err)
		}
		if err = d.ip.AddrAddDev(s.TunName, cidr6); err != nil {
			return nil, fmt.Errorf("failed to assign IPv6 to TUN %s: %s", s.TunName, err)
		}
		hasAddress = true
	}

	if !hasAddress {
		return nil, fmt.Errorf("no tunnel IP configuration: both IPv4 and IPv6 are disabled")
	}

	tunFile, err = d.ioctl.CreateTunInterface(s.TunName)
	if err != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %v", err)
	}

	return tunFile, nil
}

func (d tunDeviceManager) delete(name string) error {
	return d.ip.LinkDelete(name)
}

func (d tunDeviceManager) detectName(f *os.File) (string, error) {
	return d.ioctl.DetectTunNameFromFd(f)
}

func (d tunDeviceManager) externalInterface() (string, error) {
	return d.ip.RouteDefault()
}
