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
	if !h.IsIP() {
		t.Fatal("expected host to be IP")
	}
	ip, ok := h.IP()
	if !ok || ip != netip.MustParseAddr("192.0.2.10") {
		t.Fatalf("unexpected ip: %v, ok=%v", ip, ok)
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

func TestHostJSON_UnmarshalAndMarshal(t *testing.T) {
	var h Host
	if err := json.Unmarshal([]byte(`"ExAmPlE.org"`), &h); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if h.String() != "example.org" {
		t.Fatalf("unexpected normalized value: %q", h.String())
	}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if string(b) != `"example.org"` {
		t.Fatalf("unexpected marshaled value: %s", string(b))
	}
}

func TestHost_Endpoint_AddrPort_AndRouteIP(t *testing.T) {
	ipv6, err := NewHost("[2001:db8::1]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	endpoint, err := ipv6.Endpoint(443)
	if err != nil {
		t.Fatalf("endpoint failed: %v", err)
	}
	if endpoint != "[2001:db8::1]:443" {
		t.Fatalf("unexpected endpoint: %q", endpoint)
	}

	addrPort, err := ipv6.AddrPort(443)
	if err != nil {
		t.Fatalf("addrport failed: %v", err)
	}
	if addrPort.String() != "[2001:db8::1]:443" {
		t.Fatalf("unexpected addrport: %s", addrPort)
	}

	route, err := ipv6.RouteIP()
	if err != nil {
		t.Fatalf("route ip failed: %v", err)
	}
	if route != "2001:db8::1" {
		t.Fatalf("unexpected route ip: %q", route)
	}
}

func TestHost_MethodErrors(t *testing.T) {
	domain, err := NewHost("example.org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := domain.AddrPort(80); err == nil {
		t.Fatal("expected addrport error for domain host")
	}
	if _, err := domain.RouteIP(); err == nil {
		t.Fatal("expected routeip error for domain host")
	}

	var zero Host
	if _, err := zero.Endpoint(80); err == nil {
		t.Fatal("expected endpoint error for empty host")
	}
	if _, err := zero.ListenAddrPort(0, "::"); err == nil {
		t.Fatal("expected invalid port error")
	}
	if _, err := zero.ListenAddrPort(80, "example.org"); err == nil {
		t.Fatal("expected fallback non-ip host error")
	}
}

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

func TestHost_UnmarshalErrors(t *testing.T) {
	var h Host
	if err := json.Unmarshal([]byte(`123`), &h); err == nil {
		t.Fatal("expected error for non-string host json")
	}
	if err := json.Unmarshal([]byte(`"http://bad"`), &h); err == nil {
		t.Fatal("expected error for invalid host")
	}
}

func TestHost_ListenAddrPort_WithIP_Success(t *testing.T) {
	h, err := NewHost("192.0.2.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

func TestHost_Domain_ReturnsIP_False(t *testing.T) {
	h, err := NewHost("10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

func TestHost_Endpoint_InvalidPort(t *testing.T) {
	h, _ := NewHost("10.0.0.1")
	if _, err := h.Endpoint(0); err == nil {
		t.Fatal("expected error for port 0")
	}
	if _, err := h.Endpoint(70000); err == nil {
		t.Fatal("expected error for port 70000")
	}
}

func TestHost_AddrPort_InvalidPort(t *testing.T) {
	h, _ := NewHost("10.0.0.1")
	if _, err := h.AddrPort(0); err == nil {
		t.Fatal("expected error for port 0")
	}
}

func TestHost_NewHost_Empty(t *testing.T) {
	h, err := NewHost("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.IsZero() {
		t.Fatal("expected zero host for empty string")
	}
}

func TestHost_NewHost_Whitespace(t *testing.T) {
	h, err := NewHost("   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.IsZero() {
		t.Fatal("expected zero host for whitespace-only string")
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
		"exam_ple.com",    // underscore
		"exam!ple.com",    // exclamation
		"example-.com",    // label ending with -
		"",                // empty
		"..",              // empty labels
		"example..com",    // empty label
		"exa\tmple.com",   // tab
		"exa\nmple.com",   // newline
		"example.com/foo", // slash
		"example.com:80",  // colon
		"example.com?q=1", // question mark
		"example.com#f",   // hash
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
