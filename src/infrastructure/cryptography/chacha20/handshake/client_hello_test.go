package handshake

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

// helper to build a valid ClientHello buffer
func buildValidHello(ipVersion uint8, ip string) ([]byte, *ClientHello) {
	// generate dummy keys and nonce
	edPub := ed25519.PublicKey(bytes.Repeat([]byte{0xAA}, curvePublicKeyLength))
	curvePub := bytes.Repeat([]byte{0xBB}, curvePublicKeyLength)
	nonce := bytes.Repeat([]byte{0xCC}, nonceLength)
	ch := NewClientHello(ipVersion, ip, edPub, curvePub, nonce)
	buf, _ := ch.MarshalBinary()
	return buf, &ch
}

func TestClientHello_MarshalUnmarshal_Success(t *testing.T) {
	for _, tc := range []struct {
		version uint8
		ip      string
	}{
		{4, "192.168.0.1"},
		{6, "fe80::1"},
	} {
		buf, orig := buildValidHello(tc.version, tc.ip)
		// Unmarshal
		var got ClientHello
		if err := got.UnmarshalBinary(buf); err != nil {
			t.Fatalf("Unmarshal failed for version %d ip %s: %v", tc.version, tc.ip, err)
		}
		if got.ipVersion != orig.ipVersion {
			t.Errorf("version: got %d want %d", got.ipVersion, orig.ipVersion)
		}
		if got.ipAddress != orig.ipAddress {
			t.Errorf("ip: got %s want %s", got.ipAddress, orig.ipAddress)
		}
		if !bytes.Equal(got.edPublicKey, orig.edPublicKey) {
			t.Errorf("edPub mismatch")
		}
		if !bytes.Equal(got.curvePublicKey, orig.curvePublicKey) {
			t.Errorf("curvePub mismatch")
		}
		if !bytes.Equal(got.clientNonce, orig.clientNonce) {
			t.Errorf("nonce mismatch")
		}
	}
}

func TestClientHello_MarshalBinary_InvalidVersion(t *testing.T) {
	ch := NewClientHello(0, "192.168.0.1", nil, nil, nil)
	if _, err := ch.MarshalBinary(); err == nil {
		t.Fatal("expected error for invalid ip version, got nil")
	}
}

func TestClientHello_MarshalBinary_InvalidIPv4(t *testing.T) {
	ch := NewClientHello(4, "1.1", nil, nil, nil)
	if _, err := ch.MarshalBinary(); err == nil {
		t.Fatal("expected error for short IPv4, got nil")
	}
}

func TestClientHello_MarshalBinary_InvalidIPv6(t *testing.T) {
	ch := NewClientHello(6, "1", nil, nil, nil)
	if _, err := ch.MarshalBinary(); err == nil {
		t.Fatal("expected error for short IPv6, got nil")
	}
}

func TestClientHello_UnmarshalBinary_InvalidLength(t *testing.T) {
	buf := make([]byte, minClientHelloSizeBytes-1)
	var ch ClientHello
	if err := ch.UnmarshalBinary(buf); err == nil {
		t.Fatal("expected error for too short, got nil")
	}
	buf = make([]byte, MaxClientHelloSizeBytes+1)
	if err := ch.UnmarshalBinary(buf); err == nil {
		t.Fatal("expected error for too long, got nil")
	}
}

func TestClientHello_UnmarshalBinary_InvalidVersion(t *testing.T) {
	buf, _ := buildValidHello(4, "192.168.0.1")
	buf[0] = 7
	var ch ClientHello
	if err := ch.UnmarshalBinary(buf); err == nil {
		t.Fatal("expected error for invalid version, got nil")
	}
}

func TestClientHello_UnmarshalBinary_InvalidIPLength(t *testing.T) {
	buf, _ := buildValidHello(4, "192.168.0.1")
	// set length too large
	buf[1] = 100
	var ch ClientHello
	if err := ch.UnmarshalBinary(buf); err == nil {
		t.Fatal("expected error for invalid ip length, got nil")
	}
}
