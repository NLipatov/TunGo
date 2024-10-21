package ip

import (
	"encoding/binary"
	"fmt"
	"net"
)

func ipToUint32(ip net.IP) (uint32, error) {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0, fmt.Errorf("invalid IPv4 address")
	}
	return binary.BigEndian.Uint32(ip4), nil
}

func uint32ToIP(ip uint32) net.IP {
	result := make(net.IP, 4)
	binary.BigEndian.PutUint32(result, ip)
	return result
}

func AllocateServerIp(subnetCIDR string) (string, error) {
	ip, _, err := net.ParseCIDR(subnetCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid subnet: %v", err)
	}

	ipUint, err := ipToUint32(ip)
	if err != nil {
		return "", fmt.Errorf("invalid subnet: %v", err)
	}
	serverIp := uint32ToIP(ipUint + 1) //server ip is always a first ip of subnetwork range

	return fmt.Sprintf("%s", serverIp.String()), nil
}

func AllocateClientIp(subnetCIDR string, clientCounter int) (string, error) {
	ip, network, err := net.ParseCIDR(subnetCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid subnet: %v", err)
	}

	ipUint, err := ipToUint32(ip)
	if err != nil {
		return "", fmt.Errorf("invalid subnet: %v", err)
	}

	maskSize, bits := network.Mask.Size()
	availableAddresses := 1 << (bits - maskSize)

	// minus 2 because we don't want to use network address and broadcasting address
	if clientCounter >= availableAddresses-2 {
		return "", fmt.Errorf("client counter exceeds available addresses in the subnet")
	}

	nextIPUint := ipUint + uint32(clientCounter) + 1

	nextIP := uint32ToIP(nextIPUint)

	if nextIP.Equal(network.IP) || isBroadcastAddress(nextIP, network.Mask) {
		return "", fmt.Errorf("generated IP is invalid (network or broadcast address)")
	}

	return nextIP.String(), nil
}

func ToCIDR(subnetCIDR string, addressInSubnet string) (string, error) {
	ip := net.ParseIP(addressInSubnet)
	if ip == nil {
		return "", fmt.Errorf("invalid IP address: %s", addressInSubnet)
	}

	_, network, err := net.ParseCIDR(subnetCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid subnet: %v", err)
	}
	ones, _ := network.Mask.Size()

	return fmt.Sprintf("%s/%d", ip.String(), ones), nil
}

func isBroadcastAddress(ip net.IP, mask net.IPMask) bool {
	network := ip.Mask(mask)
	broadcast := make(net.IP, len(network))
	for i := range network {
		broadcast[i] = network[i] | ^mask[i]
	}
	return ip.Equal(broadcast)
}
