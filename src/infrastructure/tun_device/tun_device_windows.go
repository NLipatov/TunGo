package tun_device

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"tungo/settings"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
	"tungo/application"
	"tungo/settings/client_configuration"
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
		return nil, errors.New("unsupported protocol")
	}

	origPhysGateway, origPhysIP, err := getOriginalPhysicalGatewayAndInterface()
	if err != nil {
		return nil, fmt.Errorf("original route error: %w", err)
	}

	adapter, err := wintun.CreateAdapter(s.InterfaceName, "WireGuard", nil)
	if err != nil {
		return nil, fmt.Errorf("create adapter error: %w", err)
	}
	defer func() {
		if err != nil {
			_ = adapter.Close()
		}
	}()

	mtu := s.MTU
	if mtu == 0 {
		mtu = 1420
	}

	session, err := adapter.StartSession(0x800000)
	if err != nil {
		return nil, fmt.Errorf("session start error: %w", err)
	}

	device := &windowsTunDevice{
		adapter: *adapter,
		session: &session,
		name:    s.InterfaceName,
		mtu:     mtu,
	}

	tunGateway, err := computeGateway(s.InterfaceAddress)
	if err != nil {
		_ = device.Close()
		return nil, err
	}

	if err = configureWindowsTunNetsh(s.InterfaceName, s.InterfaceAddress, s.InterfaceIPCIDR, tunGateway); err != nil {
		_ = device.Close()
		return nil, err
	}

	_ = removeStaticRouteToServer(s.ConnectionIP)
	if err = addStaticRouteToServer(s.ConnectionIP, origPhysIP, origPhysGateway); err != nil {
		_ = device.Close()
		return nil, err
	}

	log.Printf("tun device created, interface %s, mtu %d", s.InterfaceName, mtu)
	return device, nil
}

func (m *windowsTunDeviceManager) DisposeTunDevices() error {
	// clear tcp settings
	_ = exec.Command("netsh", "interface", "ip", "delete", "address", "name="+m.conf.TCPSettings.InterfaceName, "addr="+m.conf.TCPSettings.InterfaceAddress).Run()
	_ = exec.Command("netsh", "interface", "ipv4", "delete", "route", "0.0.0.0/0", "name="+m.conf.TCPSettings.InterfaceName).Run()
	_ = removeStaticRouteToServer(m.conf.TCPSettings.ConnectionIP)

	// clear udp settings
	_ = exec.Command("netsh", "interface", "ip", "delete", "address", "name="+m.conf.UDPSettings.InterfaceName, "addr="+m.conf.UDPSettings.InterfaceAddress).Run()
	_ = exec.Command("netsh", "interface", "ipv4", "delete", "route", "0.0.0.0/0", "name="+m.conf.UDPSettings.InterfaceName).Run()
	_ = removeStaticRouteToServer(m.conf.UDPSettings.ConnectionIP)

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
			_, _ = windows.WaitForSingleObject(d.session.ReadWaitEvent(), windows.INFINITE)
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
	return d.adapter.Close()
}

func configureWindowsTunNetsh(interfaceName, hostIP, ipCIDR, gateway string) error {
	parts := strings.Split(ipCIDR, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid CIDR: %s", ipCIDR)
	}
	prefix, _ := strconv.Atoi(parts[1])
	mask := net.CIDRMask(prefix, 32)
	maskStr := net.IP(mask).String()

	if err := exec.Command("netsh", "interface", "ip", "set", "address", "name="+interfaceName, "static", hostIP, maskStr, gateway, "1").Run(); err != nil {
		return err
	}
	return exec.Command("netsh", "interface", "ipv4", "add", "route", "0.0.0.0/0", "name="+interfaceName, gateway, "metric=1").Run()
}

func getOriginalPhysicalGatewayAndInterface() (gateway, ifaceIP string, err error) {
	out, err := exec.Command("route", "print", "0.0.0.0").CombinedOutput()
	if err != nil {
		return
	}
	lines := strings.Split(string(out), "\n")
	bestMetric := int(^uint(0) >> 1)
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[0] == "0.0.0.0" {
			metric, _ := strconv.Atoi(fields[4])
			if metric < bestMetric {
				bestMetric = metric
				gateway, ifaceIP = fields[2], fields[3]
			}
		}
	}
	if gateway == "" || ifaceIP == "" {
		err = errors.New("default route not found")
	}
	return
}

func addStaticRouteToServer(serverIP, physIP, physGateway string) error {
	iface, err := net.InterfaceByIndex(getIfaceIndexByIP(physIP))
	if err != nil {
		return err
	}
	return exec.Command("route", "add", serverIP, "mask", "255.255.255.255", physGateway, "metric", "1", "if", strconv.Itoa(iface.Index)).Run()
}

func removeStaticRouteToServer(serverIP string) error {
	return exec.Command("route", "delete", serverIP).Run()
}

func computeGateway(ipAddr string) (string, error) {
	ip := net.ParseIP(ipAddr).To4()
	if ip == nil {
		return "", errors.New("invalid IP")
	}
	ip[3] = 1
	return ip.String(), nil
}

func getIfaceIndexByIP(ip string) int {
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if strings.Contains(addr.String(), ip) {
				return iface.Index
			}
		}
	}
	return 0
}
