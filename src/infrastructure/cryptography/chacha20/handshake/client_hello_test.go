package handshake

import (
	"bytes"
	"crypto/rand"
	"net"
	"testing"
)

func buildValidHello(t *testing.T, version uint8, ipStr string) ([]byte, ClientHello) {
	t.Helper()

	edPub := make([]byte, curvePublicKeyLength)
	if _, err := rand.Read(edPub); err != nil {
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

	ip := net.ParseIP(ipStr)
	ch := NewClientHello(version, ip, edPub, curvePub, nonce)
	buf, err := ch.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed for valid ClientHello: %v", err)
	}
	return buf, ch
}

func buildLegacyHello(t *testing.T, version uint8, ipStr string) ([]byte, ClientHello) {
	t.Helper()

	edPub := make([]byte, curvePublicKeyLength)
	if _, err := rand.Read(edPub); err != nil {
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

	ip := net.ParseIP(ipStr)
	ch := NewClientHello(version, ip, edPub, curvePub, nonce)
	buf, err := ch.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	return buf, ch
}

func TestMarshalUnmarshal_Success(t *testing.T) {
	cases := []struct {
		version uint8
		ip      string
	}{
		{4, "192.168.1.100"},
		{6, "fe80::1"},
	}
	for _, tc := range cases {
		buf, orig := buildValidHello(t, tc.version, tc.ip)

		var got ClientHello
		if err := got.UnmarshalBinary(buf); err != nil {
			t.Errorf("UnmarshalBinary failed for version=%d ip=%q: %v", tc.version, tc.ip, err)
			continue
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
	}
}

func TestMarshalBinary_InvalidVersion(t *testing.T) {
	ch := NewClientHello(0, net.ParseIP("192.168.0.1"), nil, nil, nil)
	if _, err := ch.MarshalBinary(); err == nil {
		t.Fatal("expected error for invalid IP version, got nil")
	}
}

func TestMarshalBinary_ShortIPv4(t *testing.T) {
	ch := NewClientHello(4, net.ParseIP("1.1"), nil, nil, nil)
	if _, err := ch.MarshalBinary(); err == nil {
		t.Fatal("expected error for too short IPv4, got nil")
	}
}

func TestMarshalBinary_ShortIPv6(t *testing.T) {
	ch := NewClientHello(6, net.ParseIP("1"), nil, nil, nil)
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
	var ch ClientHello
	if err := ch.UnmarshalBinary(buf); err == nil {
		t.Fatal("expected error for invalid IP version, got nil")
	}
}

func TestUnmarshalBinary_InvalidIPLength(t *testing.T) {
	buf, _ := buildValidHello(t, 4, "10.0.0.5")
	buf[1] = byte(len(buf)) // deliberately too large
	var ch ClientHello
	if err := ch.UnmarshalBinary(buf); err == nil {
		t.Fatal("expected error for invalid IP length, got nil")
	}
}

func TestMarshalUnmarshalLegacy(t *testing.T) {
	buf, orig := buildLegacyHello(t, 4, "192.168.1.1")
	var got ClientHello
	if err := got.UnmarshalBinary(buf); err != nil {
		t.Fatalf("unmarshal legacy failed: %v", err)
	}
	if !bytes.Equal(got.nonce, orig.nonce) {
		t.Errorf("nonce mismatch")
	}
}
