package server

import (
	"fmt"
	"net/netip"
	"strings"

	"tungo/application/configuration/settings"
)

func Validate(configuration Configuration) error {
	if configuration.Host != "" && strings.TrimSpace(configuration.Host) == "" {
		return fmt.Errorf("host is empty")
	}

	profiles := configuration.Profiles()
	ifNames := make(map[string]struct{}, len(profiles))
	ports := make(map[int]struct{}, len(profiles))
	subnets := make([]netip.Prefix, 0, len(profiles))

	for _, profile := range profiles {
		config := profile.Settings
		if config.TunName == "" {
			return fmt.Errorf("interface name is empty")
		}
		if _, ok := ifNames[config.TunName]; ok {
			return fmt.Errorf("duplicate interface name: %s", config.TunName)
		}
		ifNames[config.TunName] = struct{}{}

		if !profile.Enabled {
			continue
		}
		portNumber := config.Port
		if portNumber < 1 || portNumber > 65535 {
			return fmt.Errorf(
				"invalid 'Port': [%s/%s] invalid port %d: must be in 1..65535",
				config.Protocol,
				config.TunName,
				portNumber,
			)
		}
		if _, duplicate := ports[portNumber]; duplicate {
			return fmt.Errorf(
				"invalid 'Port': [%s/%s] duplicate port %d",
				config.Protocol,
				config.TunName,
				portNumber,
			)
		}
		ports[portNumber] = struct{}{}
		if config.MTU < 576 || config.MTU > 9000 {
			return fmt.Errorf(
				"invalid 'MTU': [%s/%s] invalid MTU %d: expected 576..9000",
				config.Protocol,
				config.TunName,
				config.MTU,
			)
		}
		if err := validateSubnetContainsAddr("IPv4", config.IPv4Subnet, config.IPv4, config.Protocol, config.TunName); err != nil {
			return err
		}
		subnets = append(subnets, config.IPv4Subnet)

		if config.IPv6Subnet.IsValid() {
			if err := validateSubnetContainsAddr("IPv6", config.IPv6Subnet, config.IPv6, config.Protocol, config.TunName); err != nil {
				return err
			}
			subnets = append(subnets, config.IPv6Subnet)
		}
	}

	if overlappingSubnets(subnets) {
		return fmt.Errorf("two or more subnets are overlapping")
	}

	return validateAllowedPeers(configuration.AllowedPeers)
}

func validateSubnetContainsAddr(
	family string,
	subnet netip.Prefix,
	addr netip.Addr,
	proto settings.Protocol,
	tunName string,
) error {
	if !subnet.IsValid() {
		return fmt.Errorf(
			"invalid '%sSubnet': [%s/%s] invalid CIDR %q",
			family, proto, tunName, subnet,
		)
	}
	unmapped := addr.Unmap()
	if !unmapped.IsValid() {
		return fmt.Errorf(
			"invalid '%s': [%s/%s] invalid address %q",
			family, proto, tunName, addr,
		)
	}
	if !subnet.Contains(unmapped) {
		return fmt.Errorf(
			"invalid '%s': [%s/%s] address %s not in '%sSubnet' %s",
			family, proto, tunName, addr, family, subnet,
		)
	}
	return nil
}

func overlappingSubnets(subnets []netip.Prefix) bool {
	for i := 0; i < len(subnets); i++ {
		for j := i + 1; j < len(subnets); j++ {
			a, b := subnets[i], subnets[j]
			if a.Overlaps(b) || b.Overlaps(a) {
				return true
			}
		}
	}
	return false
}

func validateAllowedPeers(peers []AllowedPeer) error {
	seenClientIDs := make(map[int]int)
	for i, peer := range peers {
		if len(peer.PublicKey) != 32 {
			return fmt.Errorf("peer %d: invalid public key length %d, expected 32", i, len(peer.PublicKey))
		}
		if peer.ClientID <= 0 {
			return fmt.Errorf("peer %d: invalid ClientID %d: must be > 0", i, peer.ClientID)
		}
		if previous, exists := seenClientIDs[peer.ClientID]; exists {
			return fmt.Errorf(
				"ClientID conflict: peer %d and peer %d both have ClientID %d",
				previous, i, peer.ClientID,
			)
		}
		seenClientIDs[peer.ClientID] = i
	}

	seenKeys := make(map[string]int)
	for i, peer := range peers {
		key := string(peer.PublicKey)
		if previous, exists := seenKeys[key]; exists {
			return fmt.Errorf("duplicate public key: peer %d and peer %d", previous, i)
		}
		seenKeys[key] = i
	}

	return nil
}
