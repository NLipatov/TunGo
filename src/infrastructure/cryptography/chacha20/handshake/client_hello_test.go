package handshake

import (
	"bytes"
	"crypto/rand"
	"net"
	"testing"
	"tungo/domain/network/ip/packet_validation"
	"tungo/infrastructure/network/ip"
)

func testPolicy() packet_validation.Policy {
	// Match the production default unless a test needs otherwise
	return packet_validation.Policy{
		AllowV4:           true,
		AllowV6:           true,
		RequirePrivate:    true,
		ForbidLoopback:    true,
		ForbidMulticast:   true,
		ForbidUnspecified: true,
		ForbidLinkLocal:   true,
		ForbidBroadcastV4: true,
	}
}

func buildValidHello(t *testing.T, version ip.Version, ipStr string) ([]byte, ClientHello) {
	t.Helper()

	edPubRaw := make([]byte, curvePublicKeyLength)
	if _, err := rand.Read(edPubRaw); err != nil {
		t.Fatalf("failed to generate ed25519 public key: %v", err)
	}
	curvePub := make([]byte, curvePublicKeyLength)
	if _, err := rand.Read(curvePub); err != nil {
		t.Fatalf("failed to generate curve public key: %v", err)
	}
	nonce := make([]byte, nonceLength)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("failed to generate nonce: %v", err)
	}

	v := packet_validation.NewDefaultIPValidator(testPolicy())

	ipAddr := net.ParseIP(ipStr)
	ch := NewClientHello(version, ipAddr, edPubRaw, curvePub, nonce, v)

	buf, err := ch.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed for valid ClientHello: %v", err)
	}
	return buf, ch
}

func TestMarshalUnmarshal_Success(t *testing.T) {
	cases := []struct {
		name    string
		version uint8
		ip      string
	}{
		{"ipv4-private", 4, "192.168.1.100"},
		{"ipv6-ula", 6, "fd12:3456:789a::1"}, // ULA; not link-local
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ipVersion := ip.Version(tc.version)
			buf, orig := buildValidHello(t, ipVersion, tc.ip)

			// Ensure the instance has a validator before UnmarshalBinary.
			got := NewEmptyClientHelloWithDefaultIPValidator()
			if err := got.UnmarshalBinary(buf); err != nil {
				t.Fatalf("UnmarshalBinary failed for version=%d ip=%q: %v", tc.version, tc.ip, err)
			}
			if got.ipVersion != orig.ipVersion {
				t.Errorf("ipVersion: got %d want %d", got.ipVersion, orig.ipVersion)
			}
			if !got.ipAddress.Equal(orig.ipAddress) {
				t.Errorf("ipAddress: got %q want %q", got.ipAddress, orig.ipAddress)
			}
			if !bytes.Equal(got.edPublicKey, orig.edPublicKey) {
				t.Error("edPublicKey mismatch")
			}
			if !bytes.Equal(got.curvePublicKey, orig.curvePublicKey) {
				t.Error("curvePublicKey mismatch")
			}
			if !bytes.Equal(got.nonce, orig.nonce) {
				t.Error("nonce mismatch")
			}
		})
	}
}

func TestMarshalBinary_InvalidVersion(t *testing.T) {
	v := packet_validation.NewDefaultIPValidator(testPolicy())
	ch := NewClientHello(0, net.ParseIP("192.168.0.1"), nil, nil, nil, v)
	if _, err := ch.MarshalBinary(); err == nil {
		t.Fatal("expected error for invalid IP version, got nil")
	}
}

func TestMarshalBinary_ShortIPv4(t *testing.T) {
	v := packet_validation.NewDefaultIPValidator(testPolicy())
	// Explicitly short IPv4 (2 bytes) instead of nil from net.ParseIP("1.1")
	shortIPv4 := net.IP{1, 1}
	ch := NewClientHello(4, shortIPv4, nil, nil, nil, v)
	if _, err := ch.MarshalBinary(); err == nil {
		t.Fatal("expected error for too short IPv4, got nil")
	}
}

func TestMarshalBinary_ShortIPv6(t *testing.T) {
	v := packet_validation.NewDefaultIPValidator(testPolicy())
	// Explicitly short IPv6 (8 bytes < 16)
	shortIPv6 := net.IP{1, 2, 3, 4, 5, 6, 7, 8}
	ch := NewClientHello(6, shortIPv6, nil, nil, nil, v)
	if _, err := ch.MarshalBinary(); err == nil {
		t.Fatal("expected error for too short IPv6, got nil")
	}
}

func TestUnmarshalBinary_TooShort(t *testing.T) {
	var ch ClientHello
	buf := make([]byte, minClientHelloSizeBytes-1)
	if err := ch.UnmarshalBinary(buf); err == nil {
		t.Fatal("expected error for buffer too short, got nil")
	}
}

func TestUnmarshalBinary_TooLong(t *testing.T) {
	var ch ClientHello
	buf := make([]byte, MaxClientHelloSizeBytes+1)
	if err := ch.UnmarshalBinary(buf); err == nil {
		t.Fatal("expected error for buffer too long, got nil")
	}
}

func TestUnmarshalBinary_InvalidVersion(t *testing.T) {
	buf, _ := buildValidHello(t, 4, "192.168.0.1")
	buf[0] = 9
	var ch ClientHello // safe: validateIP will hit "invalid version" before touching validator
	if err := ch.UnmarshalBinary(buf); err == nil {
		t.Fatal("expected error for invalid IP version, got nil")
	}
}

func TestUnmarshalBinary_InvalidIPLength(t *testing.T) {
	buf, _ := buildValidHello(t, 4, "10.0.0.5")
	// Deliberately too large: ipAddressLength + header > len(buf)
	buf[1] = byte(len(buf)) // lengthHeaderLength is 2, so this ensures overflow
	var ch ClientHello
	if err := ch.UnmarshalBinary(buf); err == nil {
		t.Fatal("expected error for invalid IP length, got nil")
	}
}
