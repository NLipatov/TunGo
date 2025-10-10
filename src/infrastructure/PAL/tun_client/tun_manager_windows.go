package tun_client

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"tungo/application/network/tun"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/windows/network_tools/netsh"
	"tungo/infrastructure/PAL/windows/tun_adapters"
	"tungo/infrastructure/settings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type PlatformTunManager struct {
	conf  client.Configuration
	netsh netsh.Contract
}

func NewPlatformTunManager(
	conf client.Configuration,
) (tun.ClientManager, error) {
	return &PlatformTunManager{
		conf:  conf,
		netsh: netsh.NewWrapper(PAL.NewExecCommander()),
	}, nil
}

func (m *PlatformTunManager) CreateTunDevice() (tun.Device, error) {
	var s settings.Settings
	switch m.conf.Protocol {
	case settings.UDP:
		s = m.conf.UDPSettings
	case settings.TCP:
		s = m.conf.TCPSettings
	case settings.WS, settings.WSS:
		s = m.conf.WSSettings
	default:
		return nil, errors.New("unsupported protocol")
	}

	origPhysGateway, origPhysIP, err := m.getOriginalPhysicalGatewayAndInterface()
	if err != nil {
		return nil, fmt.Errorf("original route error: %w", err)
	}

	adapter, err := wintun.CreateAdapter(s.InterfaceName, "TunGo", nil)
	if err != nil {
		return nil, fmt.Errorf("create adapter error: %w", err)
	}

	mtu := s.MTU
	if mtu == 0 {
		mtu = settings.SafeMTU
	}

	device, err := tun_adapters.NewWinTun(adapter)
	if err != nil {
		_ = adapter.Close()
		return nil, err
	}

	tunGateway, err := computeGateway(s.InterfaceAddress)
	if err != nil {
		_ = device.Close()
		return nil, err
	}

	if err = m.configureWindowsTunNetsh(
		s.InterfaceName,
		s.InterfaceAddress,
		s.InterfaceIPCIDR,
		tunGateway,
		mtu,
	); err != nil {
		_ = device.Close()
		return nil, err
	}

	_ = m.netsh.RouteDelete(s.ConnectionIP)
	if err = addStaticRouteToServer(s.ConnectionIP, origPhysIP, origPhysGateway); err != nil {
		_ = device.Close()
		return nil, err
	}

	// ToDo: use dns from configuration
	dnsServers := []string{"1.1.1.1", "8.8.8.8"}
	if err = m.netsh.InterfaceSetDNSServers(s.InterfaceName, dnsServers); err != nil {
		_ = device.Close()
		return nil, err
	}

	log.Printf("tun device created, interface %s, mtu %d", s.InterfaceName, mtu)
	return device, nil
}

func (m *PlatformTunManager) DisposeTunDevices() error {
	// dispose adapters by friendly names
	_ = disposeExistingTunDevices(m.conf.TCPSettings.InterfaceName)
	_ = disposeExistingTunDevices(m.conf.UDPSettings.InterfaceName)
	_ = disposeExistingTunDevices(m.conf.WSSettings.InterfaceName)

	// net configuration cleanup
	_ = m.netsh.InterfaceIPDeleteAddress(m.conf.TCPSettings.InterfaceName, m.conf.TCPSettings.InterfaceAddress)
	_ = m.netsh.InterfaceIPV4DeleteAddress(m.conf.TCPSettings.InterfaceName)
	_ = m.netsh.RouteDelete(m.conf.TCPSettings.ConnectionIP)

	_ = m.netsh.InterfaceIPDeleteAddress(m.conf.UDPSettings.InterfaceName, m.conf.UDPSettings.InterfaceAddress)
	_ = m.netsh.InterfaceIPV4DeleteAddress(m.conf.UDPSettings.InterfaceName)
	_ = m.netsh.RouteDelete(m.conf.UDPSettings.ConnectionIP)

	_ = m.netsh.InterfaceIPDeleteAddress(m.conf.WSSettings.InterfaceName, m.conf.WSSettings.InterfaceAddress)
	_ = m.netsh.InterfaceIPV4DeleteAddress(m.conf.WSSettings.InterfaceName)
	_ = m.netsh.RouteDelete(m.conf.WSSettings.ConnectionIP)

	return nil
}

func (m *PlatformTunManager) configureWindowsTunNetsh(
	interfaceName, hostIP, ipCIDR, gateway string,
	mtu int,
) error {
	parts := strings.Split(ipCIDR, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid CIDR: %s", ipCIDR)
	}
	prefix, _ := strconv.Atoi(parts[1])
	mask := net.CIDRMask(prefix, 32)
	maskStr := net.IP(mask).String()

	if err := m.netsh.InterfaceIPSetAddressStatic(interfaceName, hostIP, maskStr, gateway); err != nil {
		return err
	}

	if err := m.netsh.InterfaceIPV4AddRouteDefault(interfaceName, gateway); err != nil {
		return err
	}

	if err := m.netsh.LinkSetDevMTU(interfaceName, mtu); err != nil {
		return err
	}

	minMetric, err := getMinInterfaceMetric()
	if err != nil {
		log.Printf("warning: can't get minimal metric: %v", err)
		return nil
	}

	newMetric := minMetric - 1
	if newMetric < 1 {
		newMetric = 1
	}

	log.Printf("setting interface %s metric to %d", interfaceName, newMetric)
	return m.netsh.SetInterfaceMetric(interfaceName, newMetric)
}

func (m *PlatformTunManager) getOriginalPhysicalGatewayAndInterface() (gateway, ifaceIP string, err error) {
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
	return exec.Command("route", "add", serverIP, "mask", "255.255.255.255",
		physGateway, "metric", "1", "if", strconv.Itoa(iface.Index)).Run()
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

const (
	DigcfPresent         = 0x00000002
	DigcfDeviceinterface = 0x00000010
	ErrorNoMoreItems     = 259
	SpdrpFriendlyname    = 0x0000000C
)

type SpDeviceInterfaceData struct {
	cbSize             uint32
	InterfaceClassGuid windows.GUID
	Flags              uint32
	Reserved           uintptr
}

type SpDeviceInterfaceDetailData struct {
	cbSize     uint32
	DevicePath [1]uint16
}

type SpDevinfoData struct {
	cbSize    uint32
	ClassGuid windows.GUID
	DevInst   uint32
	Reserved  uintptr
}

var (
	modsetupapi = windows.NewLazySystemDLL("setupapi.dll")

	procSetupDiGetClassDevsW              = modsetupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces       = modsetupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetailW  = modsetupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiGetDeviceRegistryPropertyW = modsetupapi.NewProc("SetupDiGetDeviceRegistryPropertyW")
	procSetupDiDestroyDeviceInfoList      = modsetupapi.NewProc("SetupDiDestroyDeviceInfoList")
)

func EnumerateWintunAdapters(targetName string) ([]string, error) {
	wintunGUID := windows.GUID{
		Data1: 0x9C2C2E6E,
		Data2: 0x3A89,
		Data3: 0x4F8A,
		Data4: [8]byte{0xA9, 0x70, 0x82, 0x16, 0x8E, 0x8C, 0x21, 0x8A},
	}

	hDevInfo, _, err := procSetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(&wintunGUID)),
		0,
		0,
		uintptr(DigcfPresent|DigcfDeviceinterface),
	)
	if hDevInfo == uintptr(windows.InvalidHandle) || hDevInfo == 0 {
		return nil, fmt.Errorf("SetupDiGetClassDevsW failed: %v", err)
	}
	defer func(hDevInfo uintptr) {
		_, _, _ = procSetupDiDestroyDeviceInfoList.Call(hDevInfo)
	}(hDevInfo)

	var results []string
	var index uint32 = 0
	for {
		var deviceInterfaceData SpDeviceInterfaceData
		deviceInterfaceData.cbSize = uint32(unsafe.Sizeof(deviceInterfaceData))
		ret, _, err := procSetupDiEnumDeviceInterfaces.Call(
			hDevInfo,
			0,
			uintptr(unsafe.Pointer(&wintunGUID)),
			uintptr(index),
			uintptr(unsafe.Pointer(&deviceInterfaceData)),
		)
		if ret == 0 {
			var errno syscall.Errno
			if errors.As(err, &errno) && errno == ErrorNoMoreItems {
				break
			}
			index++
			continue
		}

		var requiredSize uint32
		_, _, _ = procSetupDiGetDeviceInterfaceDetailW.Call(
			hDevInfo,
			uintptr(unsafe.Pointer(&deviceInterfaceData)),
			0,
			0,
			uintptr(unsafe.Pointer(&requiredSize)),
			0,
		)
		if requiredSize == 0 {
			index++
			continue
		}

		detailDataBuffer := make([]byte, requiredSize)
		detailData := (*SpDeviceInterfaceDetailData)(unsafe.Pointer(&detailDataBuffer[0]))
		if unsafe.Sizeof(uintptr(0)) == 8 {
			detailData.cbSize = 8
		} else {
			detailData.cbSize = 5
		}

		var devInfoData SpDevinfoData
		devInfoData.cbSize = uint32(unsafe.Sizeof(devInfoData))
		ret, _, err = procSetupDiGetDeviceInterfaceDetailW.Call(
			hDevInfo,
			uintptr(unsafe.Pointer(&deviceInterfaceData)),
			uintptr(unsafe.Pointer(detailData)),
			uintptr(requiredSize),
			0,
			uintptr(unsafe.Pointer(&devInfoData)),
		)
		if ret == 0 {
			index++
			continue
		}

		devicePath := windows.UTF16PtrToString(&detailData.DevicePath[0])

		var propertyDataType uint32
		nameBuffer := make([]uint16, 256)
		var requiredSize2 uint32
		_, _, _ = procSetupDiGetDeviceRegistryPropertyW.Call(
			hDevInfo,
			uintptr(unsafe.Pointer(&devInfoData)),
			uintptr(SpdrpFriendlyname),
			uintptr(unsafe.Pointer(&propertyDataType)),
			uintptr(unsafe.Pointer(&nameBuffer[0])),
			uintptr(len(nameBuffer)*2),
			uintptr(unsafe.Pointer(&requiredSize2)),
		)
		friendlyName := windows.UTF16ToString(nameBuffer)
		if friendlyName == "" {
			friendlyName = devicePath
		}

		if friendlyName == targetName {
			results = append(results, devicePath)
		}
		index++
	}
	return results, nil
}

func disposeExistingTunDevices(targetName string) error {
	adapters, err := EnumerateWintunAdapters(targetName)
	if err != nil {
		return fmt.Errorf("failed to enumerate adapters: %w", err)
	}
	for _, devicePath := range adapters {
		adapter, err := wintun.OpenAdapter(devicePath)
		if err != nil {
			log.Printf("failed to open adapter at %s: %v", devicePath, err)
			continue
		}
		if err = adapter.Close(); err != nil {
			log.Printf("failed to close adapter at %s: %v", devicePath, err)
		} else {
			log.Printf("adapter %s closed", targetName)
		}
	}
	return nil
}

func getMinInterfaceMetric() (int, error) {
	cmd := exec.Command("netsh", "interface", "ipv4", "show", "interfaces")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to run netsh show interfaces: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	minMetric := 9999

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		metric, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}

		if metric > 0 && metric < minMetric {
			minMetric = metric
		}
	}

	if minMetric == 9999 {
		return 0, errors.New("could not determine minimal interface metric")
	}

	return minMetric, nil
}
