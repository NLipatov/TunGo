package ip

import (
	"encoding/binary"
	"fmt"
	"net"
)

func ipToUint32(ip net.IP) uint32 {
	return binary.BigEndian.Uint32(ip.To4())
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

	ipUint := ipToUint32(ip)
	serverIp := uint32ToIP(ipUint + 1) //server ip is always a first ip of subnetwork range

	return serverIp.String(), nil
}

func AllocateClientIp(subnetCIDR string, clientCounter int) (string, error) {
	ip, network, err := net.ParseCIDR(subnetCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid subnet: %v", err)
	}

	ipUint := ipToUint32(ip)
	maskSize, bits := network.Mask.Size()
	availableAddresses := 1 << (bits - maskSize)

	// minus 2 because we don't want to use network address and broadcasting address
	if clientCounter >= availableAddresses-2 {
		return "", fmt.Errorf("client counter exceeds available addresses in the subnet")
	}

	nextIPUint := ipUint + uint32(clientCounter) + 1

	nextIP := uint32ToIP(nextIPUint)

	if nextIP.Equal(network.IP) || nextIP.Equal(net.IPv4bcast) {
		return "", fmt.Errorf("generated IP is invalid (network or broadcast address)")
	}

	return nextIP.String(), nil
}
