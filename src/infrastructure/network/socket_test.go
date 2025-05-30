package network

import (
	"testing"
)

// TestNewSocket_ValidIPv4 verifies that NewSocket succeeds for a valid IPv4 and port.
func TestNewSocket_ValidIPv4(t *testing.T) {
	s, err := NewSocket("127.0.0.1", "8080")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := "127.0.0.1:8080"
	if got := s.StringAddr(); got != want {
		t.Errorf("StringAddr() = %q; want %q", got, want)
	}

	udp, err := s.UdpAddr()
	if err != nil {
		t.Fatalf("expected no error from UdpAddr(), got %v", err)
	}
	if udp.String() != want {
		t.Errorf("UdpAddr().String() = %q; want %q", udp.String(), want)
	}
}

// TestNewSocket_ValidIPv6 verifies that NewSocket succeeds for a valid IPv6 and port.
func TestNewSocket_ValidIPv6(t *testing.T) {
	s, err := NewSocket("::1", "9090")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := "[::1]:9090"
	if got := s.StringAddr(); got != want {
		t.Errorf("StringAddr() = %q; want %q", got, want)
	}

	udp, err := s.UdpAddr()
	if err != nil {
		t.Fatalf("expected no error from UdpAddr(), got %v", err)
	}
	if udp.String() != want {
		t.Errorf("UdpAddr().String() = %q; want %q", udp.String(), want)
	}
}

// TestNewSocket_InvalidIP ensures NewSocket rejects an invalid IP string.
func TestNewSocket_InvalidIP(t *testing.T) {
	if _, err := NewSocket("not.an.ip", "1234"); err == nil {
		t.Fatal("expected error for invalid IP, got nil")
	}
}

// TestNewSocket_InvalidPortNonNumeric ensures NewSocket rejects a non-numeric port.
func TestNewSocket_InvalidPortNonNumeric(t *testing.T) {
	if _, err := NewSocket("127.0.0.1", "port"); err == nil {
		t.Fatal("expected error for non-numeric port, got nil")
	}
}

// TestNewSocket_PortZero ensures NewSocket rejects port "0".
func TestNewSocket_PortZero(t *testing.T) {
	if _, err := NewSocket("127.0.0.1", "0"); err == nil {
		t.Fatal("expected error for port=0, got nil")
	}
}

// TestStringAndUdpAddr covers both StringAddr and the success path of UdpAddr.
func TestStringAndUdpAddr(t *testing.T) {
	s, err := NewSocket("192.168.1.1", "5555")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	addrStr := s.StringAddr()
	if addrStr != "192.168.1.1:5555" {
		t.Errorf("StringAddr() = %q; want %q", addrStr, "192.168.1.1:5555")
	}

	udp, err := s.UdpAddr()
	if err != nil {
		t.Fatalf("expected no error from UdpAddr(), got %v", err)
	}
	if udp.String() != addrStr {
		t.Errorf("UdpAddr().String() = %q; want %q", udp.String(), addrStr)
	}
}

// TestUdpAddr_InvalidPort verifies that UdpAddr returns an error when port is invalid.
func TestUdpAddr_InvalidPort(t *testing.T) {
	// Bypass validation by constructing Socket directly
	s := &Socket{ip: "127.0.0.1", port: "NaN"}
	if _, err := s.UdpAddr(); err == nil {
		t.Fatal("expected error from UdpAddr with invalid port, got nil")
	}
}

// TestNewSocket_PortOutOfRange ensures ports > 65535 are rejected.
func TestNewSocket_PortOutOfRange(t *testing.T) {
	if _, err := NewSocket("10.0.0.1", "65536"); err == nil {
		t.Fatal("expected error for port out of range, got nil")
	}
}

// TestNewSocket_IPv6Zone ensures IPv6 addresses with a zone specifier are rejected.
func TestNewSocket_IPv6Zone(t *testing.T) {
	if _, err := NewSocket("fe80::1%eth0", "8080"); err == nil {
		t.Fatal("expected error for IPv6 with zone specifier, got nil")
	}
}
