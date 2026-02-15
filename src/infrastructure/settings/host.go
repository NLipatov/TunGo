package settings

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

type Host string

func NewHost(raw string) (Host, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	if ip, ok := parseHostIP(trimmed); ok {
		return Host(ip.String()), nil
	}

	domain, ok := normalizeDomain(trimmed)
	if !ok {
		return "", fmt.Errorf("invalid host %q: expected IP address or domain name", raw)
	}

	return Host(domain), nil
}

func (h Host) String() string {
	return string(h)
}

func (h Host) IsZero() bool {
	return h == ""
}

func (h Host) IsIP() bool {
	_, ok := h.IP()
	return ok
}

func (h Host) Endpoint(port int) (string, error) {
	normalized, err := h.normalized()
	if err != nil {
		return "", err
	}
	if normalized.IsZero() {
		return "", fmt.Errorf("empty host")
	}
	if err := validatePort(port); err != nil {
		return "", err
	}
	return net.JoinHostPort(normalized.String(), strconv.Itoa(port)), nil
}

func (h Host) AddrPort(port int) (netip.AddrPort, error) {
	normalized, err := h.normalized()
	if err != nil {
		return netip.AddrPort{}, err
	}
	ip, ok := normalized.IP()
	if !ok {
		return netip.AddrPort{}, fmt.Errorf("host %q is not an IP address", h.String())
	}
	if err := validatePort(port); err != nil {
		return netip.AddrPort{}, err
	}
	return netip.AddrPortFrom(ip, uint16(port)), nil
}

func (h Host) ListenAddrPort(port int, defaultIP string) (netip.AddrPort, error) {
	if err := validatePort(port); err != nil {
		return netip.AddrPort{}, err
	}

	normalized, err := h.normalized()
	if err != nil {
		return netip.AddrPort{}, err
	}
	if normalized.IsZero() {
		fallback, fallbackErr := NewHost(defaultIP)
		if fallbackErr != nil {
			return netip.AddrPort{}, fallbackErr
		}
		return fallback.AddrPort(port)
	}
	return normalized.AddrPort(port)
}

// RouteIP returns an IP address suitable for route setup.
// If the host is an IP address, it is returned directly.
// If the host is a domain name, it is resolved via DNS.
func (h Host) RouteIP() (string, error) {
	normalized, err := h.normalized()
	if err != nil {
		return "", err
	}
	ip, ok := normalized.IP()
	if ok {
		return ip.String(), nil
	}
	return normalized.resolveFirstAddr(nil)
}

// RouteIPv4 returns an IPv4 address suitable for route setup.
// If the host is an IPv4 literal, it is returned directly.
// If the host is an IPv6 literal, an error is returned.
// If the host is a domain name, it is resolved and the first IPv4 result is returned.
func (h Host) RouteIPv4() (string, error) {
	normalized, err := h.normalized()
	if err != nil {
		return "", err
	}
	ip, ok := normalized.IP()
	if ok {
		if !ip.Unmap().Is4() {
			return "", fmt.Errorf("host %q is IPv6, expected IPv4", h.String())
		}
		return ip.String(), nil
	}
	filter := func(addr netip.Addr) bool { return addr.Unmap().Is4() }
	return normalized.resolveFirstAddr(filter)
}

// RouteIPv6 returns an IPv6 address suitable for route setup.
// If the host is an IPv6 literal, it is returned directly.
// If the host is an IPv4 literal, an error is returned.
// If the host is a domain name, it is resolved and the first IPv6 result is returned.
func (h Host) RouteIPv6() (string, error) {
	normalized, err := h.normalized()
	if err != nil {
		return "", err
	}
	ip, ok := normalized.IP()
	if ok {
		if ip.Unmap().Is4() {
			return "", fmt.Errorf("host %q is IPv4, expected IPv6", h.String())
		}
		return ip.String(), nil
	}
	filter := func(addr netip.Addr) bool { return !addr.Unmap().Is4() }
	return normalized.resolveFirstAddr(filter)
}

// resolveFirstAddr resolves the domain and returns the first address matching filter.
// If filter is nil, any address is accepted.
// Returned addresses are always normalized via Unmap() for consistency with parseHostIP.
func (h Host) resolveFirstAddr(filter func(netip.Addr) bool) (string, error) {
	domain, domainOk := h.Domain()
	if !domainOk {
		return "", fmt.Errorf("host %q is neither an IP address nor a valid domain", h.String())
	}
	addrs, resolveErr := net.LookupHost(domain)
	if resolveErr != nil || len(addrs) == 0 {
		return "", fmt.Errorf("failed to resolve host %q: %v", domain, resolveErr)
	}
	for _, a := range addrs {
		ip, err := netip.ParseAddr(a)
		if err != nil {
			continue
		}
		if filter == nil || filter(ip) {
			return ip.Unmap().String(), nil
		}
	}
	return "", fmt.Errorf("no matching address family found resolving host %q", h.String())
}

func (h Host) IP() (netip.Addr, bool) {
	ip, ok := parseHostIP(h.String())
	return ip, ok
}

func (h Host) Domain() (string, bool) {
	if h.IsZero() {
		return "", false
	}
	if h.IsIP() {
		return "", false
	}
	domain, ok := normalizeDomain(h.String())
	if !ok {
		return "", false
	}
	return domain, true
}

func (h Host) normalized() (Host, error) {
	return NewHost(h.String())
}

func (h Host) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.String())
}

func (h *Host) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("host must be JSON string: %w", err)
	}

	normalized, err := NewHost(raw)
	if err != nil {
		return err
	}

	*h = normalized
	return nil
}

func parseHostIP(raw string) (netip.Addr, bool) {
	ip, err := netip.ParseAddr(strings.Trim(raw, "[]"))
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}

func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}
	return nil
}

func normalizeDomain(raw string) (string, bool) {
	domain := strings.ToLower(strings.TrimSpace(raw))
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" || len(domain) > 253 {
		return "", false
	}
	if strings.ContainsAny(domain, " \t\n\r/:?#[]@\\") {
		return "", false
	}
	labels := strings.Split(domain, ".")
	for _, label := range labels {
		if !isValidDomainLabel(label) {
			return "", false
		}
	}
	return domain, true
}

func isValidDomainLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 {
		return false
	}
	if label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, c := range label {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		return false
	}
	return true
}
