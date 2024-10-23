package confgen

import (
	"etha-tunnel/network/ip"
	"etha-tunnel/settings"
	"etha-tunnel/settings/client"
	"etha-tunnel/settings/server"
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// Generate generates new client configuration
func Generate() (*client.Conf, error) {
	serverConf, err := (&server.Conf{}).Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration: %s", err)
	}

	serverIpAddr, addressResolutionError := getServerIpString()
	if addressResolutionError != nil {
		if serverConf.FallbackServerAddress == "" {
			return nil, fmt.Errorf("failed to resolve server IP and no fallback address provided in server configuration: %s", addressResolutionError)
		}
		serverIpAddr = serverConf.FallbackServerAddress
	}

	var serverTCPAddress string
	// for IPv6, port must be handled in different way
	if strings.Contains(serverIpAddr, ":") {
		serverTCPAddress = fmt.Sprintf("[%s]%s", serverIpAddr, serverConf.TCPSettings.ConnectionPort)
	} else {
		serverTCPAddress = fmt.Sprintf("%s%s", serverIpAddr, serverConf.TCPSettings.ConnectionPort)
	}

	IncrementedClientCounter := serverConf.ClientCounter + 1
	clientIfIp, err := ip.AllocateClientIp(serverConf.TCPSettings.InterfaceIPCIDR, IncrementedClientCounter)

	serverConf.ClientCounter = IncrementedClientCounter
	err = serverConf.RewriteConf()
	if err != nil {
		return nil, err
	}

	conf := client.Conf{
		TCPSettings: settings.ConnectionSettings{
			InterfaceName:    serverConf.TCPSettings.InterfaceName,
			InterfaceIPCIDR:  serverConf.TCPSettings.InterfaceIPCIDR,
			InterfaceAddress: clientIfIp,
			ConnectionPort:   serverConf.TCPSettings.ConnectionPort,
		},
		IfName:                    "ethatun0",
		IfIP:                      clientIfIp,
		ServerTCPAddress:          serverTCPAddress,
		Ed25519PublicKey:          serverConf.Ed25519PublicKey,
		TCPWriteChannelBufferSize: 1000,
	}

	return &conf, nil
}

func getServerIpString() (string, error) {
	v4Addr, err := getV4Addr()
	if err == nil {
		return v4Addr, nil
	}

	v6Addr, err := getV6Addr()
	if err == nil {
		return v6Addr, nil
	}

	return "", fmt.Errorf("failed to determine server IP address")
}

func getV4Addr() (string, error) {
	cmd := exec.Command("sh", "-c", `ip -4 addr | sed -ne 's|^.* inet \([^/]*\)/.* scope global.*$|\1|p' | awk '{print $1}' | head -1`)
	res, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to resolve ipV4 address")
	}

	ip := strings.Trim(string(res), "\n")

	v4Valid := isValidIPv4(ip)
	if !v4Valid {
		return "", fmt.Errorf("not a valid IPv4 address")
	}

	return ip, nil
}

func getV6Addr() (string, error) {
	cmd := exec.Command("sh", "-c", `ip -6 addr | sed -ne 's|^.* inet6 \([^/]*\)/.* scope global.*$|\1|p' | head -1`)
	res, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to resolve ipV6 address")
	}

	ip := strings.Trim(string(res), "\n")

	v6Valid := isValidIPv6(ip)
	if !v6Valid {
		return "", fmt.Errorf("not a valid IPv6 address")
	}

	return ip, nil
}

func isValidIPv4(ip string) bool {
	if ip == "" {
		return false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// Check if it's a valid v6 address
	if parsedIP.To4() == nil {
		return false
	}

	// Check if ip is not local or special
	privateIPBlocks := []*net.IPNet{
		// Loopback
		{IP: net.IPv4(127, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
		// Private networks
		{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
		{IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},
		{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)},
		// Link-local (APIPA)
		{IP: net.IPv4(169, 254, 0, 0), Mask: net.CIDRMask(16, 32)},
	}

	for _, block := range privateIPBlocks {
		if block.Contains(parsedIP) {
			return false
		}
	}

	return true
}

func isValidIPv6(ip string) bool {
	if ip == "" {
		return false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// Check if it's a valid v6 address
	if parsedIP.To16() == nil || parsedIP.To4() != nil {
		return false
	}

	// Check if ip is not local or special
	privateIPBlocks := []*net.IPNet{
		// Link-local
		{IP: net.ParseIP("fe80::"), Mask: net.CIDRMask(10, 128)},
		// Unique Local Addresses (ULA)
		{IP: net.ParseIP("fc00::"), Mask: net.CIDRMask(7, 128)},
		// Loopback
		{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)},
	}

	for _, block := range privateIPBlocks {
		if block.Contains(parsedIP) {
			return false
		}
	}

	return true
}
