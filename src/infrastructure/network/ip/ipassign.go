package ip

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

// AllocateServerIP returns the first usable address in the given subnet.
// Works for both IPv4 and IPv6.
func AllocateServerIP(subnet netip.Prefix) (string, error) {
	if !subnet.IsValid() {
		return "", fmt.Errorf("invalid subnet: %s", subnet)
	}
	addr := subnet.Addr()
	if addr.Is4() {
		arr := addr.As4()
		base := binary.BigEndian.Uint32(arr[:])
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], base+1)
		return netip.AddrFrom4(b).String(), nil
	}
	// IPv6: increment the low 64 bits by 1
	b := addr.As16()
	lo := binary.BigEndian.Uint64(b[8:])
	lo++
	binary.BigEndian.PutUint64(b[8:], lo)
	return netip.AddrFrom16(b).String(), nil
}

// AllocateClientIP returns the address for the given clientID (1-based)
// within the subnet, skipping the network base and the server address (base+1).
// Works for both IPv4 and IPv6.
func AllocateClientIP(subnet netip.Prefix, clientCounter int) (netip.Addr, error) {
	if !subnet.IsValid() {
		return netip.Addr{}, fmt.Errorf("invalid subnet: %s", subnet)
	}
	addr := subnet.Addr()
	if addr.Is4() {
		return allocateClientIPv4(addr, subnet, clientCounter)
	}
	return allocateClientIPv6(addr, subnet, clientCounter)
}

func allocateClientIPv4(addr netip.Addr, subnet netip.Prefix, clientCounter int) (netip.Addr, error) {
	arr := addr.As4()
	base := binary.BigEndian.Uint32(arr[:])
	ones, bits := subnet.Bits(), 32
	total := 1 << (bits - ones)
	if clientCounter < 1 || clientCounter >= total-2 {
		return netip.Addr{}, fmt.Errorf("client counter out of range (must be 1..%d): %d", total-3, clientCounter)
	}
	next := base + 1 + uint32(clientCounter)
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], next)
	clientIP := netip.AddrFrom4(b)
	if clientIP == addr || next == base+uint32(total-1) {
		return netip.Addr{}, fmt.Errorf("generated IP is invalid (network or broadcast address): %s", clientIP)
	}
	return clientIP, nil
}

func allocateClientIPv6(addr netip.Addr, subnet netip.Prefix, clientCounter int) (netip.Addr, error) {
	if clientCounter < 1 {
		return netip.Addr{}, fmt.Errorf("client counter out of range (must be >= 1): %d", clientCounter)
	}
	// offset = clientCounter + 1 (skip the network base; +1 is the server)
	offset := uint64(clientCounter) + 1
	b := addr.As16()
	lo := binary.BigEndian.Uint64(b[8:])
	lo += offset
	binary.BigEndian.PutUint64(b[8:], lo)
	return netip.AddrFrom16(b), nil
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
