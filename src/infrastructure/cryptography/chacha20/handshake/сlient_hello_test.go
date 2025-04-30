package handshake

import (
	"bytes"
	"errors"
	"testing"
)

func makeValidClientHello() *ClientHello {
	ed := make([]byte, curvePublicKeyLength)
	curve := make([]byte, curvePublicKeyLength)
	nonce := make([]byte, nonceLength)
	for i := range ed {
		ed[i] = byte(i)
		curve[i] = byte(i + 16)
	}
	for i := range nonce {
		nonce[i] = byte(i + 32)
	}
	ch, _ := NewClientHello(4, "192.168.0.1", ed, curve, nonce)
	return ch
}

func TestMarshalUnmarshal_Success(t *testing.T) {
	ch := makeValidClientHello()
	data, err := ch.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	var other ClientHello
	err = other.UnmarshalBinary(data)
	if err != nil {
		t.Fatalf("UnmarshalBinary failed: %v", err)
	}
	if other.IpVersion != ch.IpVersion || other.IpAddress != ch.IpAddress {
		t.Errorf("decoded header mismatch: got %v %v, want %v %v", other.IpVersion, other.IpAddress, ch.IpVersion, ch.IpAddress)
	}
	if !bytes.Equal(other.Ed25519PubKey, ch.Ed25519PubKey) {
		t.Error("Ed25519PubKey mismatch")
	}
	if !bytes.Equal(other.CurvePubKey, ch.CurvePubKey) {
		t.Error("CurvePubKey mismatch")
	}
	if !bytes.Equal(other.ClientNonce, ch.ClientNonce) {
		t.Error("ClientNonce mismatch")
	}
}

func TestMarshal_ErrorVersion(t *testing.T) {
	ch := &ClientHello{IpVersion: 7, IpAddress: "any", Ed25519PubKey: make([]byte, curvePublicKeyLength), CurvePubKey: make([]byte, curvePublicKeyLength), ClientNonce: make([]byte, nonceLength)}
	_, err := ch.MarshalBinary()
	if !errors.Is(err, ErrInvalidIPVersion) {
		t.Errorf("expected ErrInvalidIPVersion, got %v", err)
	}
}

func TestMarshal_ErrorIPLength(t *testing.T) {
	ch := &ClientHello{IpVersion: 4, IpAddress: "1.1", Ed25519PubKey: make([]byte, curvePublicKeyLength), CurvePubKey: make([]byte, curvePublicKeyLength), ClientNonce: make([]byte, nonceLength)}
	_, err := ch.MarshalBinary()
	if !errors.Is(err, ErrInvalidIPAddrLength) {
		t.Errorf("expected ErrInvalidIPAddrLength, got %v", err)
	}
}

func TestUnmarshal_ErrorLength(t *testing.T) {
	var ch ClientHello
	err := ch.UnmarshalBinary(make([]byte, minClientHelloSizeBytes-1))
	if !errors.Is(err, ErrInvalidMessageLength) {
		t.Errorf("expected ErrInvalidMessageLength, got %v", err)
	}
}

func TestUnmarshal_ErrorVersion(t *testing.T) {
	// construct minimal-length with bad version
	data := make([]byte, minClientHelloSizeBytes)
	data[0] = 9
	data[1] = 0
	err := new(ClientHello).UnmarshalBinary(data)
	if !errors.Is(err, ErrInvalidIPVersion) {
		t.Errorf("expected ErrInvalidIPVersion, got %v", err)
	}
}

func TestUnmarshal_ErrorIPLenField(t *testing.T) {
	// valid version, but length too large
	data := make([]byte, minClientHelloSizeBytes)
	data[0] = 4
	data[1] = uint8(len(data))
	err := new(ClientHello).UnmarshalBinary(data)
	if !errors.Is(err, ErrInvalidIPAddrLength) {
		t.Errorf("expected ErrInvalidIPAddrLength, got %v", err)
	}
}
