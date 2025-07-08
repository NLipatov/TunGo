package handshake

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io"
	"tungo/application"
)

type ServerHandshake struct {
	conn      application.ConnectionAdapter
	sharedKey []byte
}

func NewServerHandshake(conn application.ConnectionAdapter, sharedKey []byte) ServerHandshake {
	return ServerHandshake{
		conn:      conn,
		sharedKey: sharedKey,
	}
}

func (h *ServerHandshake) ReceiveClientHello() (ClientHello, error) {
	var first [1]byte
	if _, err := io.ReadFull(h.conn, first[:]); err != nil {
		return ClientHello{}, err
	}

	// plain hello starts with ip version
	if first[0] == 4 || first[0] == 6 {
		var hdr [1]byte
		if _, err := io.ReadFull(h.conn, hdr[:]); err != nil {
			return ClientHello{}, err
		}
		ipLen := int(hdr[0])
		rest := make([]byte, ipLen+curvePublicKeyLength+curvePublicKeyLength+nonceLength)
		if _, err := io.ReadFull(h.conn, rest); err != nil {
			return ClientHello{}, err
		}

		data := append([]byte{first[0], hdr[0]}, rest...)
		var hello ClientHello
		if err := hello.UnmarshalBinary(data); err != nil {
			return ClientHello{}, err
		}
		return hello, nil
	}

	// obfuscated hello
	padLenBuf := make([]byte, 1)
	if _, err := io.ReadFull(h.conn, padLenBuf); err != nil {
		return ClientHello{}, err
	}
	padLen := int(padLenBuf[0])
	initial := padLen + minClientHelloSizeBytes + hmacLength
	buf := make([]byte, initial)
	if _, err := io.ReadFull(h.conn, buf); err != nil {
		return ClientHello{}, err
	}

	ipLen := int(buf[padLen+1])
	helloLen := lengthHeaderLength + ipLen + curvePublicKeyLength + curvePublicKeyLength + nonceLength
	total := padLen + helloLen + hmacLength
	if total > initial {
		extra := make([]byte, total-initial)
		if _, err := io.ReadFull(h.conn, extra); err != nil {
			return ClientHello{}, err
		}
		buf = append(buf, extra...)
	}

	padding := buf[:padLen]
	helloBytes := buf[padLen : padLen+helloLen]
	receivedMac := buf[padLen+helloLen : total]

	mac := hmac.New(sha256.New, h.sharedKey)
	mac.Write(padding)
	mac.Write(helloBytes)
	if !hmac.Equal(mac.Sum(nil), receivedMac) {
		return ClientHello{}, fmt.Errorf("invalid hmac")
	}

	var hello ClientHello
	if err := hello.UnmarshalBinary(helloBytes); err != nil {
		return ClientHello{}, err
	}
	return hello, nil
}

func (h *ServerHandshake) SendServerHello(
	c Crypto,
	serverPrivateKey ed25519.PrivateKey,
	serverNonce []byte,
	curvePublic,
	clientNonce []byte) error {
	serverDataToSign := append(append(curvePublic, serverNonce...), clientNonce...)
	serverSignature := c.Sign(serverPrivateKey, serverDataToSign)
	serverHello := NewServerHello(serverSignature, serverNonce, curvePublic)
	marshalledServerHello, marshalErr := serverHello.MarshalBinary()
	if marshalErr != nil {
		return marshalErr
	}

	_, writeErr := h.conn.Write(marshalledServerHello)
	return writeErr
}

func (h *ServerHandshake) VerifyClientSignature(c Crypto, hello ClientHello, serverNonce []byte) error {
	clientSignatureBuf := make([]byte, 64)
	if _, err := io.ReadFull(h.conn, clientSignatureBuf); err != nil {
		return err
	}

	var clientSignature Signature
	unmarshalErr := clientSignature.UnmarshalBinary(clientSignatureBuf)
	if unmarshalErr != nil {
		return unmarshalErr
	}

	// Verify client signature
	if !c.Verify(hello.edPublicKey, append(append(hello.curvePublicKey, hello.nonce...), serverNonce...), clientSignature.data) {
		return fmt.Errorf("client failed signature verification")
	}

	return nil
}
