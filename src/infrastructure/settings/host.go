package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// Host is a composite server address: it may carry a domain name, an IPv4 address,
// an IPv6 address, or any combination. All fields are optional; a zero Host has none set.
type Host struct {
	domain string
	ipv4   netip.Addr
	ipv6   netip.Addr
}

var lookupHostContext = func(ctx context.Context, domain string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, domain)
}

// IPHost creates a Host from a string that must be a valid IP address.
func IPHost(raw string) (Host, error) {
	ip, ok := parseHostIP(strings.TrimSpace(raw))
	if !ok {
		return Host{}, fmt.Errorf("expected IP address, got %q", raw)
	}
	return hostFromIP(ip), nil
}

// DomainHost creates a Host from a string that must be a valid domain name.
func DomainHost(raw string) (Host, error) {
	trimmed := strings.TrimSpace(raw)
	if _, isIP := parseHostIP(trimmed); isIP {
		return Host{}, fmt.Errorf("expected domain name, got IP %q", raw)
	}
	domain, ok := normalizeDomain(trimmed)
	if !ok {
		return Host{}, fmt.Errorf("expected domain name, got %q", raw)
	}
	return Host{domain: domain}, nil
}

// NewHost parses a single value: IPv4 → sets ipv4, IPv6 → sets ipv6, domain → sets domain.
// Empty string returns a zero Host.
func NewHost(raw string) (Host, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Host{}, nil
	}

	if ip, ok := parseHostIP(trimmed); ok {
		return hostFromIP(ip), nil
	}

	domain, ok := normalizeDomain(trimmed)
	if !ok {
		return Host{}, fmt.Errorf("invalid host %q: expected IP address or domain name", raw)
	}

	return Host{domain: domain}, nil
}

// hostFromIP places an IP into the correct field based on address family.
func hostFromIP(ip netip.Addr) Host {
	if ip.Unmap().Is4() {
		return Host{ipv4: ip}
	}
	return Host{ipv6: ip}
}

// WithIPv4 returns a new Host with the ipv4 field set.
func (h Host) WithIPv4(addr netip.Addr) Host {
	h.ipv4 = addr.Unmap()
	return h
}

// WithIPv6 returns a new Host with the ipv6 field set.
func (h Host) WithIPv6(addr netip.Addr) Host {
	h.ipv6 = addr
	return h
}

func (h Host) String() string {
	if h.domain != "" {
		return h.domain
	}
	if h.ipv4.IsValid() {
		return h.ipv4.String()
	}
	if h.ipv6.IsValid() {
		return h.ipv6.String()
	}
	return ""
}

func (h Host) IsZero() bool {
	return h.domain == "" && !h.ipv4.IsValid() && !h.ipv6.IsValid()
}

func (h Host) IsIP() bool {
	return h.ipv4.IsValid() || h.ipv6.IsValid()
}

func (h Host) HasIPv4() bool {
	return h.ipv4.IsValid()
}

func (h Host) HasIPv6() bool {
	return h.ipv6.IsValid()
}

// IP returns ipv4 if set, else ipv6.
func (h Host) IP() (netip.Addr, bool) {
	if h.ipv4.IsValid() {
		return h.ipv4, true
	}
	if h.ipv6.IsValid() {
		return h.ipv6, true
	}
	return netip.Addr{}, false
}

func (h Host) IPv4() (netip.Addr, bool) {
	return h.ipv4, h.ipv4.IsValid()
}

func (h Host) IPv6() (netip.Addr, bool) {
	return h.ipv6, h.ipv6.IsValid()
}

func (h Host) Domain() (string, bool) {
	return h.domain, h.domain != ""
}

// Endpoint returns domain:port > ipv4:port > [ipv6]:port.
func (h Host) Endpoint(port int) (string, error) {
	if h.IsZero() {
		return "", fmt.Errorf("empty host")
	}
	if err := validatePort(port); err != nil {
		return "", err
	}
	return net.JoinHostPort(h.String(), strconv.Itoa(port)), nil
}

// IPv6Endpoint returns [ipv6]:port specifically.
func (h Host) IPv6Endpoint(port int) (string, error) {
	if !h.ipv6.IsValid() {
		return "", fmt.Errorf("host has no IPv6 address")
	}
	if err := validatePort(port); err != nil {
		return "", err
	}
	return net.JoinHostPort(h.ipv6.String(), strconv.Itoa(port)), nil
}

// AddrPort returns ipv4 preferred, else ipv6.
func (h Host) AddrPort(port int) (netip.AddrPort, error) {
	ip, ok := h.IP()
	if !ok {
		return netip.AddrPort{}, fmt.Errorf("host %q is not an IP address", h.String())
	}
	if err := validatePort(port); err != nil {
		return netip.AddrPort{}, err
	}
	return netip.AddrPortFrom(ip, uint16(port)), nil
}

// IPv6AddrPort returns ipv6 specifically.
func (h Host) IPv6AddrPort(port int) (netip.AddrPort, error) {
	if !h.ipv6.IsValid() {
		return netip.AddrPort{}, fmt.Errorf("host has no IPv6 address")
	}
	if err := validatePort(port); err != nil {
		return netip.AddrPort{}, err
	}
	return netip.AddrPortFrom(h.ipv6, uint16(port)), nil
}

func (h Host) ListenAddrPort(port int, defaultIP string) (netip.AddrPort, error) {
	if err := validatePort(port); err != nil {
		return netip.AddrPort{}, err
	}
	if h.IsZero() {
		fallback, fallbackErr := IPHost(defaultIP)
		if fallbackErr != nil {
			return netip.AddrPort{}, fallbackErr
		}
		return fallback.AddrPort(port)
	}
	return h.AddrPort(port)
}

// RouteIP returns an IP address suitable for route setup.
// If the host has an IP address, it is returned directly.
// If the host is a domain name, it is resolved via DNS.
func (h Host) RouteIP() (string, error) {
	return h.RouteIPContext(context.Background())
}

// RouteIPContext returns an IP address suitable for route setup using context-bounded DNS resolution.
func (h Host) RouteIPContext(ctx context.Context) (string, error) {
	if ip, ok := h.IP(); ok {
		return ip.String(), nil
	}
	return h.resolveFirstAddr(ctx, nil)
}

// RouteIPv4 returns an IPv4 address suitable for route setup.
// If the host has an ipv4 field, it is returned directly.
// If the host is a domain name, it is resolved and the first IPv4 result is returned.
func (h Host) RouteIPv4() (string, error) {
	return h.RouteIPv4Context(context.Background())
}

// RouteIPv4Context returns an IPv4 address suitable for route setup using context-bounded DNS resolution.
func (h Host) RouteIPv4Context(ctx context.Context) (string, error) {
	if h.ipv4.IsValid() {
		return h.ipv4.String(), nil
	}
	if h.ipv6.IsValid() && h.domain == "" {
		return "", fmt.Errorf("host %q is IPv6, expected IPv4", h.String())
	}
	filter := func(addr netip.Addr) bool { return addr.Unmap().Is4() }
	return h.resolveFirstAddr(ctx, filter)
}

// RouteIPv6 returns an IPv6 address suitable for route setup.
// If the host has an ipv6 field, it is returned directly.
// If the host is a domain name, it is resolved and the first IPv6 result is returned.
func (h Host) RouteIPv6() (string, error) {
	return h.RouteIPv6Context(context.Background())
}

// RouteIPv6Context returns an IPv6 address suitable for route setup using context-bounded DNS resolution.
func (h Host) RouteIPv6Context(ctx context.Context) (string, error) {
	if h.ipv6.IsValid() {
		return h.ipv6.String(), nil
	}
	if h.ipv4.IsValid() && h.domain == "" {
		return "", fmt.Errorf("host %q is IPv4, expected IPv6", h.String())
	}
	filter := func(addr netip.Addr) bool { return !addr.Unmap().Is4() }
	return h.resolveFirstAddr(ctx, filter)
}

// resolveFirstAddr resolves the domain and returns the first address matching filter.
// If filter is nil, any address is accepted.
// Returned addresses are always normalized via Unmap() for consistency with parseHostIP.
func (h Host) resolveFirstAddr(ctx context.Context, filter func(netip.Addr) bool) (string, error) {
	domain, domainOk := h.Domain()
	if !domainOk {
		return "", fmt.Errorf("host %q is neither an IP address nor a valid domain", h.String())
	}
	if ctx == nil {
		ctx = context.Background()
	}
	addrs, resolveErr := lookupHostContext(ctx, domain)
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

// hostJSON is the wire format for Host.
type hostJSON struct {
	Domain string `json:"Domain,omitzero"`
	IPv4   string `json:"IPv4,omitzero"`
	IPv6   string `json:"IPv6,omitzero"`
}

func (h Host) MarshalJSON() ([]byte, error) {
	obj := hostJSON{}
	if h.domain != "" {
		obj.Domain = h.domain
	}
	if h.ipv4.IsValid() {
		obj.IPv4 = h.ipv4.String()
	}
	if h.ipv6.IsValid() {
		obj.IPv6 = h.ipv6.String()
	}
	return json.Marshal(obj)
}

func (h *Host) UnmarshalJSON(data []byte) error {
	var obj hostJSON
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("invalid Host JSON: %w", err)
	}

	var result Host
	if obj.Domain != "" {
		domain, ok := normalizeDomain(obj.Domain)
		if !ok {
			return fmt.Errorf("invalid domain %q in Host", obj.Domain)
		}
		result.domain = domain
	}
	if obj.IPv4 != "" {
		ip, ok := parseHostIP(obj.IPv4)
		if !ok || !ip.Unmap().Is4() {
			return fmt.Errorf("invalid IPv4 %q in Host", obj.IPv4)
		}
		result.ipv4 = ip
	}
	if obj.IPv6 != "" {
		ip, ok := parseHostIP(obj.IPv6)
		if !ok || ip.Unmap().Is4() {
			return fmt.Errorf("invalid IPv6 %q in Host", obj.IPv6)
		}
		result.ipv6 = ip
	}
	*h = result
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
