package tun_device

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
	"tungo/application"
	"tungo/settings"
	"tungo/settings/client_configuration"

	"golang.zx2c4.com/wintun"
)

type windowsTunDeviceManager struct {
	conf client_configuration.Configuration
}

func newPlatformTunConfigurator(conf client_configuration.Configuration) (application.PlatformTunConfigurator, error) {
	return &windowsTunDeviceManager{conf: conf}, nil
}

func (m *windowsTunDeviceManager) CreateTunDevice() (application.TunDevice, error) {
	var s settings.ConnectionSettings
	switch m.conf.Protocol {
	case settings.UDP:
		s = m.conf.UDPSettings
	case settings.TCP:
		s = m.conf.TCPSettings
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}

	// wintun.dll is expected to be in PATH directory, for example in C:\Windows\System32
	adapter, err := wintun.CreateAdapter(s.InterfaceName, "WireGuard", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Wintun adapter: %v", err)
	}

	forcedMTU := 1420
	if s.MTU > 0 {
		forcedMTU = s.MTU
	}

	session, err := adapter.StartSession(0x800000)
	if err != nil {
		_ = adapter.Close()
		return nil, fmt.Errorf("failed to start session: %v", err)
	}

	device := &windowsTunDevice{
		adapter: *adapter,
		session: &session,
		name:    s.InterfaceName,
		mtu:     forcedMTU,
	}

	gateway, err := computeGateway(s.InterfaceAddress)
	if err != nil {
		_ = device.Close()
		return nil, fmt.Errorf("failed to compute gateway: %v", err)
	}

	if err := configureWindowsTunNetsh(s.InterfaceName, s.InterfaceAddress, s.InterfaceIPCIDR, gateway); err != nil {
		_ = device.Close()
		return nil, fmt.Errorf("failed to configure TUN: %v", err)
	}

	fmt.Printf("created Wintun interface: %s with MTU %d\n", s.InterfaceName, forcedMTU)
	return device, nil
}

func (m *windowsTunDeviceManager) DisposeTunDevices() error {
	return nil
}

type windowsTunDevice struct {
	adapter wintun.Adapter
	session *wintun.Session
	name    string
	mtu     int
}

func (d *windowsTunDevice) Read(data []byte) (int, error) {
	for {
		packet, err := d.session.ReceivePacket()
		if err == nil {
			n := copy(data, packet)
			d.session.ReleaseReceivePacket(packet)
			return n, nil
		}
		if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
			handle := d.session.ReadWaitEvent()
			_, _ = windows.WaitForSingleObject(handle, windows.INFINITE)
			continue
		}
		return 0, err
	}
}

func (d *windowsTunDevice) Write(data []byte) (int, error) {
	packet, err := d.session.AllocateSendPacket(len(data))
	if err != nil {
		return 0, err
	}
	copy(packet, data)
	d.session.SendPacket(packet)
	return len(data), nil
}

func (d *windowsTunDevice) Close() error {
	d.session.End()
	_ = d.adapter.Close()
	return nil
}

func configureWindowsTunNetsh(interfaceName, hostIP, ipCIDR, gateway string) error {
	parts := strings.Split(ipCIDR, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid CIDR format: %s", ipCIDR)
	}
	prefix, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid prefix: %v", err)
	}
	mask := net.CIDRMask(prefix, 32)
	maskStr := fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])

	cmd := exec.Command("netsh", "interface", "ip", "set", "address",
		"name="+interfaceName, "static", hostIP, maskStr, gateway, "1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("netsh error: %v, output: %s", err, out)
	}

	cmd = exec.Command("route", "add", "0.0.0.0", "mask", "0.0.0.0", gateway, "metric", "1")
	out, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("route error: %v, output: %s", err, out)
	}
	return nil
}

func computeGateway(ipAddr string) (string, error) {
	parts := strings.Split(ipAddr, ".")
	if len(parts) != 4 {
		return "", fmt.Errorf("invalid IP address: %s", ipAddr)
	}
	parts[3] = "1"
	return strings.Join(parts, "."), nil
}
