package settings

import (
	"encoding/json"
	"net/netip"
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
