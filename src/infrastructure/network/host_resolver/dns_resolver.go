package host_resolver

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	"tungo/application/configuration/settings"
)

// ResolveIP returns the preferred configured IP, resolving the domain when needed.
func ResolveIP(ctx context.Context, host settings.Host) (string, error) {
	if host.IPv4 != "" {
		return parseConfiguredIP(host.IPv4, true)
	}
	if host.IPv6 != "" {
		return parseConfiguredIP(host.IPv6, false)
	}
	return resolveDomain(ctx, host.Domain, "ip")
}

// ResolveIPv4 returns the configured or resolved IPv4 address.
func ResolveIPv4(ctx context.Context, host settings.Host) (string, error) {
	if host.IPv4 != "" {
		return parseConfiguredIP(host.IPv4, true)
	}
	if host.Domain == "" && host.IPv6 != "" {
		return "", fmt.Errorf("host %q is IPv6, expected IPv4", host.IPv6)
	}
	return resolveDomain(ctx, host.Domain, "ip4")
}

// ResolveIPv6 returns the configured or resolved IPv6 address.
func ResolveIPv6(ctx context.Context, host settings.Host) (string, error) {
	if host.IPv6 != "" {
		return parseConfiguredIP(host.IPv6, false)
	}
	if host.Domain == "" && host.IPv4 != "" {
		return "", fmt.Errorf("host %q is IPv4, expected IPv6", host.IPv4)
	}
	return resolveDomain(ctx, host.Domain, "ip6")
}

func parseConfiguredIP(raw string, wantIPv4 bool) (string, error) {
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return "", fmt.Errorf("invalid IP address %q: %w", raw, err)
	}
	ip = ip.Unmap()
	if ip.Is4() != wantIPv4 {
		return "", fmt.Errorf("IP address %q has unexpected address family", raw)
	}
	return ip.String(), nil
}

func resolveDomain(ctx context.Context, domain, network string) (string, error) {
	if domain == "" {
		return "", fmt.Errorf("server host is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, network, domain)
	if err != nil || len(addrs) == 0 {
		return "", fmt.Errorf("failed to resolve host %q: %v", domain, err)
	}
	return addrs[0].Unmap().String(), nil
}
