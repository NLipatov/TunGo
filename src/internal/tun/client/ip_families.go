package client

import "tungo/internal/config/settings"

func hasIPv4(active settings.Settings) bool {
	return active.IPv4Subnet.IsValid() && active.IPv4Subnet.Addr().Unmap().Is4()
}

func hasIPv6(active settings.Settings) bool {
	return active.IPv6Subnet.IsValid() && active.IPv6Subnet.Addr().Unmap().Is6()
}
