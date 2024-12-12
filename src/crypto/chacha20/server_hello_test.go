package chacha20

import (
	"bytes"
	"testing"
)

func TestServerHello_Read(t *testing.T) {
	data := make([]byte, signatureLength+nonceLength+curvePublicKeyLength)
	copy(data[:signatureLength], bytes.Repeat([]byte{0x01}, signatureLength))
	copy(data[signatureLength:signatureLength+nonceLength], bytes.Repeat([]byte{0x02}, nonceLength))
	copy(data[signatureLength+nonceLength:], bytes.Repeat([]byte{0x03}, curvePublicKeyLength))

	sH := &ServerHello{}
	result, err := sH.Read(data)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !bytes.Equal(result.Signature, data[:signatureLength]) {
		t.Errorf("Signature does not match. Expected %v, got %v", data[:signatureLength], result.Signature)
	}
	if !bytes.Equal(result.Nonce, data[signatureLength:signatureLength+nonceLength]) {
		t.Errorf("Nonce does not match. Expected %v, got %v", data[signatureLength:signatureLength+nonceLength], result.Nonce)
	}
	if !bytes.Equal(result.CurvePublicKey, data[signatureLength+nonceLength:]) {
		t.Errorf("CurvePublicKey does not match. Expected %v, got %v", data[signatureLength+nonceLength:], result.CurvePublicKey)
	}
}

func TestServerHello_Read_InvalidData(t *testing.T) {
	data := make([]byte, signatureLength+nonceLength+10) // Invalid length

	sH := &ServerHello{}
	_, err := sH.Read(data)
	if err == nil {
		t.Fatalf("Expected error for invalid data length, got nil")
	}
}

func TestServerHello_Write(t *testing.T) {
	signature := bytes.Repeat([]byte{0x01}, signatureLength)
	nonce := bytes.Repeat([]byte{0x02}, nonceLength)
	curvePublicKey := bytes.Repeat([]byte{0x03}, curvePublicKeyLength)

	sH := &ServerHello{}
	result, err := sH.Write(&signature, &nonce, &curvePublicKey)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expected := append(append(signature, nonce...), curvePublicKey...)
	if !bytes.Equal(*result, expected) {
		t.Errorf("Result does not match. Expected %v, got %v", expected, *result)
	}
}

func TestServerHello_Write_InvalidSignature(t *testing.T) {
	signature := bytes.Repeat([]byte{0x01}, signatureLength-1) // Invalid length
	nonce := bytes.Repeat([]byte{0x02}, nonceLength)
	curvePublicKey := bytes.Repeat([]byte{0x03}, curvePublicKeyLength)

	sH := &ServerHello{}
	_, err := sH.Write(&signature, &nonce, &curvePublicKey)
	if err == nil {
		t.Fatalf("Expected error for invalid signature length, got nil")
	}
}

func TestServerHello_Write_InvalidNonce(t *testing.T) {
	signature := bytes.Repeat([]byte{0x01}, signatureLength)
	nonce := bytes.Repeat([]byte{0x02}, nonceLength-1) // Invalid length
	curvePublicKey := bytes.Repeat([]byte{0x03}, curvePublicKeyLength)

	sH := &ServerHello{}
	_, err := sH.Write(&signature, &nonce, &curvePublicKey)
	if err == nil {
		t.Fatalf("Expected error for invalid nonce length, got nil")
	}
}

func TestServerHello_Write_InvalidCurvePublicKey(t *testing.T) {
	signature := bytes.Repeat([]byte{0x01}, signatureLength)
	nonce := bytes.Repeat([]byte{0x02}, nonceLength)
	curvePublicKey := bytes.Repeat([]byte{0x03}, curvePublicKeyLength-1) // Invalid length

	sH := &ServerHello{}
	_, err := sH.Write(&signature, &nonce, &curvePublicKey)
	if err == nil {
		t.Fatalf("Expected error for invalid curve public key length, got nil")
	}
}
