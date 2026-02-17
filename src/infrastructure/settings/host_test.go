package settings

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
)

func TestNewHost_IPv4(t *testing.T) {
	h, err := NewHost("192.0.2.10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.HasIPv4() {
		t.Fatal("expected host to have IPv4")
	}
	ip, ok := h.IPv4()
	if !ok || ip != netip.MustParseAddr("192.0.2.10") {
		t.Fatalf("unexpected ipv4: %v, ok=%v", ip, ok)
	}
	if h.HasIPv6() {
		t.Fatal("IPv4 host should not have IPv6")
	}
}

func TestNewHost_IPv6(t *testing.T) {
	h, err := NewHost("2001:db8::1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.HasIPv6() {
		t.Fatal("expected host to have IPv6")
	}
	ip, ok := h.IPv6()
	if !ok || ip != netip.MustParseAddr("2001:db8::1") {
		t.Fatalf("unexpected ipv6: %v, ok=%v", ip, ok)
	}
	if h.HasIPv4() {
		t.Fatal("IPv6 host should not have IPv4")
	}
}

func TestNewHost_Domain(t *testing.T) {
	h, err := NewHost("API.EXAMPLE.COM")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.IsIP() {
		t.Fatal("expected host to be domain")
	}
	domain, ok := h.Domain()
	if !ok || domain != "api.example.com" {
		t.Fatalf("unexpected domain: %q, ok=%v", domain, ok)
	}
}

func TestNewHost_Invalid(t *testing.T) {
	_, err := NewHost("https://example.com")
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestNewHost_Empty(t *testing.T) {
	h, err := NewHost("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.IsZero() {
		t.Fatal("expected zero host for empty string")
	}
}

func TestNewHost_Whitespace(t *testing.T) {
	h, err := NewHost("   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.IsZero() {
		t.Fatal("expected zero host for whitespace-only string")
	}
}

func TestHost_WithIPv4(t *testing.T) {
	h, _ := NewHost("example.com")
	h2 := h.WithIPv4(netip.MustParseAddr("192.0.2.1"))
	if !h2.HasIPv4() {
		t.Fatal("expected IPv4 after WithIPv4")
	}
	ip, _ := h2.IPv4()
	if ip != netip.MustParseAddr("192.0.2.1") {
		t.Fatalf("unexpected IPv4: %v", ip)
	}
	// Original must be unchanged.
	if h.HasIPv4() {
		t.Fatal("original host should not have IPv4")
	}
	// Domain preserved.
	domain, ok := h2.Domain()
	if !ok || domain != "example.com" {
		t.Fatal("domain should be preserved after WithIPv4")
	}
}

func TestHost_WithIPv6(t *testing.T) {
	h, _ := NewHost("192.0.2.1")
	h2 := h.WithIPv6(netip.MustParseAddr("2001:db8::1"))
	if !h2.HasIPv6() {
		t.Fatal("expected IPv6 after WithIPv6")
	}
	ip, _ := h2.IPv6()
	if ip != netip.MustParseAddr("2001:db8::1") {
		t.Fatalf("unexpected IPv6: %v", ip)
	}
	// Original must be unchanged.
	if h.HasIPv6() {
		t.Fatal("original host should not have IPv6")
	}
	// IPv4 preserved.
	if !h2.HasIPv4() {
		t.Fatal("IPv4 should be preserved after WithIPv6")
	}
}

func TestHost_IsZero(t *testing.T) {
	var zero Host
	if !zero.IsZero() {
		t.Fatal("expected IsZero for empty host")
	}
	h, _ := NewHost("10.0.0.1")
	if h.IsZero() {
		t.Fatal("expected non-zero host")
	}
}

func TestHost_IsIP(t *testing.T) {
	h4, _ := NewHost("10.0.0.1")
	if !h4.IsIP() {
		t.Fatal("IPv4 host should be IP")
	}
	h6, _ := NewHost("2001:db8::1")
	if !h6.IsIP() {
		t.Fatal("IPv6 host should be IP")
	}
	hd, _ := NewHost("example.com")
	if hd.IsIP() {
		t.Fatal("domain host should not be IP")
	}
}

func TestHost_IP_PrefersIPv4(t *testing.T) {
	h, _ := NewHost("192.0.2.1")
	h = h.WithIPv6(netip.MustParseAddr("2001:db8::1"))
	ip, ok := h.IP()
	if !ok || ip != netip.MustParseAddr("192.0.2.1") {
		t.Fatalf("IP() should prefer IPv4, got %v", ip)
	}
}

func TestHost_IP_FallsBackToIPv6(t *testing.T) {
	h, _ := NewHost("2001:db8::1")
	ip, ok := h.IP()
	if !ok || ip != netip.MustParseAddr("2001:db8::1") {
		t.Fatalf("IP() should fall back to IPv6, got %v", ip)
	}
}

func TestHost_Domain_ReturnsIP_False(t *testing.T) {
	h, _ := NewHost("10.0.0.1")
	domain, ok := h.Domain()
	if ok || domain != "" {
		t.Fatalf("expected Domain()=(\"\", false) for IP host, got (%q, %v)", domain, ok)
	}
}

func TestHost_Domain_Empty_False(t *testing.T) {
	var zero Host
	domain, ok := zero.Domain()
	if ok || domain != "" {
		t.Fatalf("expected Domain()=(\"\", false) for empty host, got (%q, %v)", domain, ok)
	}
}

func TestHost_String(t *testing.T) {
	// domain > ipv4 > ipv6 precedence
	h, _ := NewHost("example.com")
	h = h.WithIPv4(netip.MustParseAddr("1.2.3.4"))
	h = h.WithIPv6(netip.MustParseAddr("2001:db8::1"))
	if h.String() != "example.com" {
		t.Fatalf("expected domain in String(), got %q", h.String())
	}

	h2, _ := NewHost("1.2.3.4")
	h2 = h2.WithIPv6(netip.MustParseAddr("2001:db8::1"))
	if h2.String() != "1.2.3.4" {
		t.Fatalf("expected ipv4 in String(), got %q", h2.String())
	}

	h3, _ := NewHost("2001:db8::1")
	if h3.String() != "2001:db8::1" {
		t.Fatalf("expected ipv6 in String(), got %q", h3.String())
	}
}

func TestHost_Endpoint_IPv4(t *testing.T) {
	h, _ := NewHost("10.0.0.1")
	ep, err := h.Endpoint(443)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep != "10.0.0.1:443" {
		t.Fatalf("unexpected endpoint: %q", ep)
	}
}

func TestHost_Endpoint_IPv6(t *testing.T) {
	h, _ := NewHost("[2001:db8::1]")
	ep, err := h.Endpoint(443)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep != "[2001:db8::1]:443" {
		t.Fatalf("unexpected endpoint: %q", ep)
	}
}

func TestHost_Endpoint_Domain(t *testing.T) {
	h, _ := NewHost("example.org")
	ep, err := h.Endpoint(8080)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep != "example.org:8080" {
		t.Fatalf("unexpected endpoint: %q", ep)
	}
}

func TestHost_Endpoint_InvalidPort(t *testing.T) {
	h, _ := NewHost("10.0.0.1")
	if _, err := h.Endpoint(0); err == nil {
		t.Fatal("expected error for port 0")
	}
	if _, err := h.Endpoint(70000); err == nil {
		t.Fatal("expected error for port 70000")
	}
}

func TestHost_Endpoint_EmptyHost(t *testing.T) {
	var zero Host
	if _, err := zero.Endpoint(80); err == nil {
		t.Fatal("expected endpoint error for empty host")
	}
}

func TestHost_IPv6Endpoint(t *testing.T) {
	h, _ := NewHost("192.0.2.1")
	h = h.WithIPv6(netip.MustParseAddr("2001:db8::1"))

	ep, err := h.IPv6Endpoint(443)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep != "[2001:db8::1]:443" {
		t.Fatalf("unexpected IPv6 endpoint: %q", ep)
	}
}

func TestHost_IPv6Endpoint_NoIPv6_Error(t *testing.T) {
	h, _ := NewHost("192.0.2.1")
	_, err := h.IPv6Endpoint(443)
	if err == nil {
		t.Fatal("expected error for host without IPv6")
	}
}

func TestHost_AddrPort(t *testing.T) {
	h, _ := NewHost("[2001:db8::1]")
	ap, err := h.AddrPort(443)
	if err != nil {
		t.Fatalf("addrport failed: %v", err)
	}
	if ap.String() != "[2001:db8::1]:443" {
		t.Fatalf("unexpected addrport: %s", ap)
	}
}

func TestHost_AddrPort_InvalidPort(t *testing.T) {
	h, _ := NewHost("10.0.0.1")
	if _, err := h.AddrPort(0); err == nil {
		t.Fatal("expected error for port 0")
	}
}

func TestHost_AddrPort_EmptyHost_Error(t *testing.T) {
	var zero Host
	if _, err := zero.AddrPort(80); err == nil {
		t.Fatal("expected error for empty host AddrPort")
	}
}

func TestHost_AddrPort_DomainHost_Error(t *testing.T) {
	h, _ := NewHost("example.org")
	if _, err := h.AddrPort(80); err == nil {
		t.Fatal("expected addrport error for domain host")
	}
}

func TestHost_IPv6AddrPort(t *testing.T) {
	h, _ := NewHost("192.0.2.1")
	h = h.WithIPv6(netip.MustParseAddr("2001:db8::1"))

	ap, err := h.IPv6AddrPort(443)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ap.String() != "[2001:db8::1]:443" {
		t.Fatalf("unexpected IPv6AddrPort: %s", ap)
	}
}

func TestHost_IPv6AddrPort_NoIPv6_Error(t *testing.T) {
	h, _ := NewHost("192.0.2.1")
	_, err := h.IPv6AddrPort(443)
	if err == nil {
		t.Fatal("expected error for host without IPv6")
	}
}

func TestHost_ListenAddrPort_WithIP_Success(t *testing.T) {
	h, _ := NewHost("192.0.2.1")
	ap, err := h.ListenAddrPort(443, "0.0.0.0")
	if err != nil {
		t.Fatalf("ListenAddrPort failed: %v", err)
	}
	if ap.String() != "192.0.2.1:443" {
		t.Fatalf("unexpected addr:port: %s", ap)
	}
}

func TestHost_ListenAddrPort_ZeroHost_FallsBackToDefault(t *testing.T) {
	var zero Host
	ap, err := zero.ListenAddrPort(80, "0.0.0.0")
	if err != nil {
		t.Fatalf("ListenAddrPort with fallback failed: %v", err)
	}
	if ap.String() != "0.0.0.0:80" {
		t.Fatalf("unexpected addr:port: %s", ap)
	}
}

func TestHost_ListenAddrPort_ZeroHost_InvalidFallback(t *testing.T) {
	var zero Host
	_, err := zero.ListenAddrPort(80, "not-a-valid-@-host")
	if err == nil {
		t.Fatal("expected error for invalid fallback")
	}
}

func TestHost_ListenAddrPort_DomainHost_Error(t *testing.T) {
	h, _ := NewHost("example.org")
	if _, err := h.ListenAddrPort(80, "0.0.0.0"); err == nil {
		t.Fatal("expected error for domain host in ListenAddrPort")
	}
}

func TestHost_ListenAddrPort_InvalidPort(t *testing.T) {
	var zero Host
	if _, err := zero.ListenAddrPort(0, "::"); err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestHost_RouteIP_IPv4(t *testing.T) {
	h, _ := NewHost("192.168.1.1")
	route, err := h.RouteIP()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if route != "192.168.1.1" {
		t.Fatalf("unexpected route: %q", route)
	}
}

func TestHost_RouteIP_IPv6(t *testing.T) {
	h, _ := NewHost("2001:db8::1")
	route, err := h.RouteIP()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if route != "2001:db8::1" {
		t.Fatalf("unexpected route: %q", route)
	}
}

func TestHost_RouteIP_EmptyHost(t *testing.T) {
	var zero Host
	if _, err := zero.RouteIP(); err == nil {
		t.Fatal("expected error for empty host RouteIP")
	}
}

func TestHost_RouteIPv4_IPv4Literal(t *testing.T) {
	h, _ := NewHost("192.168.1.1")
	route, err := h.RouteIPv4()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if route != "192.168.1.1" {
		t.Fatalf("expected 192.168.1.1, got %q", route)
	}
}

func TestHost_RouteIPv4_IPv6Only_Error(t *testing.T) {
	h, _ := NewHost("2001:db8::1")
	_, err := h.RouteIPv4()
	if err == nil {
		t.Fatal("expected error for IPv6-only literal in RouteIPv4")
	}
}

func TestHost_RouteIPv6_IPv6Literal(t *testing.T) {
	h, _ := NewHost("2001:db8::1")
	route, err := h.RouteIPv6()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if route != "2001:db8::1" {
		t.Fatalf("expected 2001:db8::1, got %q", route)
	}
}

func TestHost_RouteIPv6_IPv4Only_Error(t *testing.T) {
	h, _ := NewHost("192.168.1.1")
	_, err := h.RouteIPv6()
	if err == nil {
		t.Fatal("expected error for IPv4-only literal in RouteIPv6")
	}
}

func TestHost_RouteIPv4_EmptyHost_Error(t *testing.T) {
	var zero Host
	_, err := zero.RouteIPv4()
	if err == nil {
		t.Fatal("expected error for empty host RouteIPv4")
	}
}

func TestHost_RouteIPv6_EmptyHost_Error(t *testing.T) {
	var zero Host
	_, err := zero.RouteIPv6()
	if err == nil {
		t.Fatal("expected error for empty host RouteIPv6")
	}
}

func TestHost_RouteIPv4_UnresolvableDomain_Error(t *testing.T) {
	h, _ := NewHost("this-does-not-exist.invalid")
	_, err := h.RouteIPv4()
	if err == nil {
		t.Fatal("expected error for unresolvable domain in RouteIPv4")
	}
}

func TestHost_RouteIPv6_UnresolvableDomain_Error(t *testing.T) {
	h, _ := NewHost("this-does-not-exist.invalid")
	_, err := h.RouteIPv6()
	if err == nil {
		t.Fatal("expected error for unresolvable domain in RouteIPv6")
	}
}

func TestHost_RouteIP_UnresolvableDomain_Error(t *testing.T) {
	h, _ := NewHost("this-domain-does-not-exist.invalid")
	_, err := h.RouteIP()
	if err == nil {
		t.Fatal("expected routeip error for unresolvable domain host")
	}
}

// --- JSON tests ---

func TestHost_JSON_Object_IPv4Only(t *testing.T) {
	h, _ := NewHost("192.0.2.10")
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !strings.Contains(string(b), `"IPv4":"192.0.2.10"`) {
		t.Fatalf("expected IPv4 in JSON, got %s", string(b))
	}
	if strings.Contains(string(b), `"IPv6"`) {
		t.Fatalf("unexpected IPv6 in JSON: %s", string(b))
	}
	if strings.Contains(string(b), `"Domain"`) {
		t.Fatalf("unexpected Domain in JSON: %s", string(b))
	}

	var h2 Host
	if err := json.Unmarshal(b, &h2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if h != h2 {
		t.Fatalf("roundtrip mismatch: %v != %v", h, h2)
	}
}

func TestHost_JSON_Object_Composite(t *testing.T) {
	h, _ := NewHost("example.com")
	h = h.WithIPv4(netip.MustParseAddr("192.0.2.10"))
	h = h.WithIPv6(netip.MustParseAddr("2001:db8::1"))

	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"Domain":"example.com"`) {
		t.Fatalf("expected Domain in JSON: %s", s)
	}
	if !strings.Contains(s, `"IPv4":"192.0.2.10"`) {
		t.Fatalf("expected IPv4 in JSON: %s", s)
	}
	if !strings.Contains(s, `"IPv6":"2001:db8::1"`) {
		t.Fatalf("expected IPv6 in JSON: %s", s)
	}

	var h2 Host
	if err := json.Unmarshal(b, &h2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if h != h2 {
		t.Fatalf("roundtrip mismatch: %v != %v", h, h2)
	}
}

func TestHost_JSON_UnmarshalErrors(t *testing.T) {
	var h Host
	if err := json.Unmarshal([]byte(`123`), &h); err == nil {
		t.Fatal("expected error for numeric host json")
	}
	if err := json.Unmarshal([]byte(`"some-string"`), &h); err == nil {
		t.Fatal("expected error for string host json")
	}
}

func TestHost_JSON_Object_InvalidIPv4(t *testing.T) {
	var h Host
	if err := json.Unmarshal([]byte(`{"IPv4":"not-an-ip"}`), &h); err == nil {
		t.Fatal("expected error for invalid IPv4 in object")
	}
}

func TestHost_JSON_Object_InvalidIPv6(t *testing.T) {
	var h Host
	if err := json.Unmarshal([]byte(`{"IPv6":"not-an-ip"}`), &h); err == nil {
		t.Fatal("expected error for invalid IPv6 in object")
	}
}

func TestHost_JSON_Object_InvalidDomain(t *testing.T) {
	var h Host
	if err := json.Unmarshal([]byte(`{"Domain":"http://bad"}`), &h); err == nil {
		t.Fatal("expected error for invalid domain in object")
	}
}

func TestHost_JSON_ZeroHost_EmptyObject(t *testing.T) {
	var h Host
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if string(b) != `{}` {
		t.Fatalf("expected empty object for zero host, got %s", string(b))
	}
}

// --- Constructor tests ---

func TestIPHost_IPv4(t *testing.T) {
	h, err := IPHost("192.0.2.10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.HasIPv4() {
		t.Fatal("expected IPv4")
	}
}

func TestIPHost_IPv6(t *testing.T) {
	h, err := IPHost("2001:db8::1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.HasIPv6() {
		t.Fatal("expected IPv6")
	}
}

func TestIPHost_Domain_Error(t *testing.T) {
	_, err := IPHost("example.com")
	if err == nil {
		t.Fatal("expected error for domain in IPHost")
	}
}

func TestIPHost_Empty_Error(t *testing.T) {
	_, err := IPHost("")
	if err == nil {
		t.Fatal("expected error for empty string in IPHost")
	}
}

func TestDomainHost_Valid(t *testing.T) {
	h, err := DomainHost("Example.COM")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	domain, ok := h.Domain()
	if !ok || domain != "example.com" {
		t.Fatalf("unexpected domain: %q, ok=%v", domain, ok)
	}
}

func TestDomainHost_IP_Error(t *testing.T) {
	_, err := DomainHost("192.0.2.1")
	if err == nil {
		t.Fatal("expected error for IP in DomainHost")
	}
}

func TestDomainHost_Empty_Error(t *testing.T) {
	_, err := DomainHost("")
	if err == nil {
		t.Fatal("expected error for empty string in DomainHost")
	}
}

// --- Helper / internal tests ---

func TestHost_NormalizationAndDomainValidation(t *testing.T) {
	ip, ok := parseHostIP("[::ffff:192.0.2.55]")
	if !ok {
		t.Fatal("expected mapped IPv4 to parse")
	}
	if ip != netip.MustParseAddr("192.0.2.55") {
		t.Fatalf("expected unmapped IPv4, got %s", ip)
	}

	if _, ok := normalizeDomain("bad domain"); ok {
		t.Fatal("expected invalid domain with whitespace")
	}
	if _, ok := normalizeDomain(strings.Repeat("a", 64) + ".example.com"); ok {
		t.Fatal("expected invalid domain with label length >63")
	}
	if _, ok := normalizeDomain("-example.com"); ok {
		t.Fatal("expected invalid domain starting with '-'")
	}
	if domain, ok := normalizeDomain("Example.COM."); !ok || domain != "example.com" {
		t.Fatalf("expected normalized domain, got %q ok=%v", domain, ok)
	}
}

func TestNormalizeDomain_TooLong(t *testing.T) {
	long := strings.Repeat("a.", 127) + "a" // > 253 chars
	if _, ok := normalizeDomain(long); ok {
		t.Fatal("expected invalid for domain >253 chars")
	}
}

func TestNormalizeDomain_InvalidChars(t *testing.T) {
	invalid := []string{
		"exam_ple.com",
		"exam!ple.com",
		"example-.com",
		"",
		"..",
		"example..com",
		"exa\tmple.com",
		"exa\nmple.com",
		"example.com/foo",
		"example.com:80",
		"example.com?q=1",
		"example.com#f",
	}
	for _, s := range invalid {
		if _, ok := normalizeDomain(s); ok {
			t.Errorf("expected normalizeDomain(%q) to fail", s)
		}
	}
}

func TestIsValidDomainLabel_InvalidChars(t *testing.T) {
	if isValidDomainLabel("") {
		t.Fatal("expected false for empty label")
	}
	if isValidDomainLabel(strings.Repeat("a", 64)) {
		t.Fatal("expected false for label >63 chars")
	}
	if isValidDomainLabel("abc_def") {
		t.Fatal("expected false for underscore in label")
	}
	if isValidDomainLabel("-abc") {
		t.Fatal("expected false for leading dash")
	}
	if isValidDomainLabel("abc-") {
		t.Fatal("expected false for trailing dash")
	}
	if !isValidDomainLabel("a-b-c") {
		t.Fatal("expected true for valid label with dashes")
	}
	if !isValidDomainLabel("a123") {
		t.Fatal("expected true for alphanumeric label")
	}
}

func TestNormalizeDomain_SingleLabel(t *testing.T) {
	domain, ok := normalizeDomain("localhost")
	if !ok || domain != "localhost" {
		t.Fatalf("expected 'localhost', got %q ok=%v", domain, ok)
	}
}

func TestNormalizeDomain_BackslashInvalid(t *testing.T) {
	if _, ok := normalizeDomain(`exam\ple.com`); ok {
		t.Fatal("expected invalid for backslash")
	}
}

func TestNormalizeDomain_AtSignInvalid(t *testing.T) {
	if _, ok := normalizeDomain("user@example.com"); ok {
		t.Fatal("expected invalid for @ sign")
	}
}

func TestNormalizeDomain_BracketInvalid(t *testing.T) {
	if _, ok := normalizeDomain("[example].com"); ok {
		t.Fatal("expected invalid for brackets")
	}
}

func TestHost_RouteIPv6_CompositeHost_WithIPv6(t *testing.T) {
	// A composite host with ipv4 + ipv6 should return ipv6 from RouteIPv6.
	h, _ := NewHost("192.0.2.1")
	h = h.WithIPv6(netip.MustParseAddr("2001:db8::1"))

	route, err := h.RouteIPv6()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if route != "2001:db8::1" {
		t.Fatalf("expected 2001:db8::1, got %q", route)
	}
}

func TestHost_RouteIPv4_CompositeHost_WithIPv4(t *testing.T) {
	// A composite host with ipv4 + ipv6 should return ipv4 from RouteIPv4.
	h, _ := NewHost("192.0.2.1")
	h = h.WithIPv6(netip.MustParseAddr("2001:db8::1"))

	route, err := h.RouteIPv4()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if route != "192.0.2.1" {
		t.Fatalf("expected 192.0.2.1, got %q", route)
	}
}
