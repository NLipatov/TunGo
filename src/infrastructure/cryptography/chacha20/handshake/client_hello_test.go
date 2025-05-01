package handshake

import (
	"bytes"
	"crypto/rand"
	"testing"
)

// helper builds a valid ClientHello and its serialized form
func buildValidHello(t *testing.T, version uint8, ip string) ([]byte, ClientHello) {
	t.Helper()

	// dummy keys and nonce
	edPub := make([]byte, curvePublicKeyLength)
	rand.Read(edPub)
	curvePub := make([]byte, curvePublicKeyLength)
	rand.Read(curvePub)
	nonce := make([]byte, nonceLength)
	rand.Read(nonce)

	ch := NewClientHello(version, ip, edPub, curvePub, nonce)
	buf, err := ch.MarshalBinary()
	if err != nil {
		t.Fatalf("failed to marshal valid ClientHello: %v", err)
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
		if got.ipAddress != orig.ipAddress {
			t.Errorf("ipAddress: got %q want %q", got.ipAddress, orig.ipAddress)
		}
		if !bytes.Equal(got.edPublicKey, orig.edPublicKey) {
			t.Errorf("edPublicKey mismatch")
		}
		if !bytes.Equal(got.curvePublicKey, orig.curvePublicKey) {
			t.Errorf("curvePublicKey mismatch")
		}
		if !bytes.Equal(got.clientNonce, orig.clientNonce) {
			t.Errorf("clientNonce mismatch")
		}
	}
}

func TestMarshalBinary_InvalidVersion(t *testing.T) {
	ch := NewClientHello(0, "192.168.0.1", nil, nil, nil)
	if _, err := ch.MarshalBinary(); err == nil {
		t.Fatal("expected error for invalid IP version, got nil")
	}
}

func TestMarshalBinary_ShortIPv4(t *testing.T) {
	ch := NewClientHello(4, "1.1", nil, nil, nil)
	if _, err := ch.MarshalBinary(); err == nil {
		t.Fatal("expected error for too short IPv4, got nil")
	}
}

func TestMarshalBinary_ShortIPv6(t *testing.T) {
	ch := NewClientHello(6, "1", nil, nil, nil)
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
	// set IP length byte too large
	buf[1] = byte(len(buf)) // definitely > actual length
	var ch ClientHello
	if err := ch.UnmarshalBinary(buf); err == nil {
		t.Fatal("expected error for invalid IP length, got nil")
	}
}
