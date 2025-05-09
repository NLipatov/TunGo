package network

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

// AllocateServerIp returns the first usable IPv4 address in the given subnet.
func AllocateServerIp(subnetCIDR string) (string, error) {
	pref, err := netip.ParsePrefix(subnetCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid subnet: %w", err)
	}
	addr := pref.Addr()
	if !addr.Is4() {
		return "", fmt.Errorf("only IPv4 supported: %s", subnetCIDR)
	}
	// convert base network address to uint32 and add 1
	arr := addr.As4() // [4]byte
	base := binary.BigEndian.Uint32(arr[:])
	// construct server IP as AddrFrom4
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], base+1)
	server := netip.AddrFrom4(b)
	return server.String(), nil
}

// AllocateClientIp returns the IPv4 address for the given client index (0-based)
// within the subnet, skipping network and broadcast addresses.
func AllocateClientIp(subnetCIDR string, clientCounter int) (string, error) {
	pref, err := netip.ParsePrefix(subnetCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid subnet: %w", err)
	}
	addr := pref.Addr()
	if !addr.Is4() {
		return "", fmt.Errorf("only IPv4 supported: %s", subnetCIDR)
	}
	arr := addr.As4()
	base := binary.BigEndian.Uint32(arr[:])
	ones, bits := pref.Bits(), 32
	total := 1 << (bits - ones)
	if clientCounter < 0 || clientCounter >= total-2 {
		return "", fmt.Errorf("client counter exceeds available addresses in the subnet: %d", clientCounter)
	}
	// calculate client IP
	next := base + 1 + uint32(clientCounter)
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], next)
	clientIP := netip.AddrFrom4(b)
	// ensure not network or broadcast
	if clientIP == addr || next == base+uint32(total-1) {
		return "", fmt.Errorf("generated IP is invalid (network or broadcast address): %s", clientIP)
	}
	return clientIP.String(), nil
}

// ToCIDR combines an IP address with the mask length from the given subnet.
func ToCIDR(subnetCIDR string, addressInSubnet string) (string, error) {
	pref, err := netip.ParsePrefix(subnetCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid subnet: %w", err)
	}
	rip, err := netip.ParseAddr(addressInSubnet)
	if err != nil {
		return "", fmt.Errorf("invalid IP address: %w", err)
	}
	// preserve mask bits
	return fmt.Sprintf("%s/%d", rip.String(), pref.Bits()), nil
}
