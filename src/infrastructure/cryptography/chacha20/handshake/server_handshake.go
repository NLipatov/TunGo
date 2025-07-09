package handshake

import (
	"crypto/ed25519"
	"fmt"
	"io"
	"tungo/application"
)

type ServerHandshake struct {
	conn                  application.ConnectionAdapter
	clientHelloObfuscated bool
	clientHelloEncrypted  bool
	enc                   Encrypter
}

func NewServerHandshake(conn application.ConnectionAdapter, enc Encrypter) ServerHandshake {
	return ServerHandshake{
		conn: conn,
		enc:  enc,
	}
}

func (h *ServerHandshake) ReceiveClientHello() (ClientHello, error) {
	buf := make([]byte, MaxClientHelloSizeBytes+paddingLengthHeaderBytes+maxPaddingLength+hmacLength)
	n, readErr := h.conn.Read(buf)
	if readErr != nil && readErr != io.EOF {
		return ClientHello{}, readErr
	}
	buf = buf[:n]

	plainEnc, encrypted, err := h.enc.Decrypt(buf)
	if err != nil {
		return ClientHello{}, err
	}

	obfs := Obfuscator{}
	plain, obfuscated, err := obfs.Deobfuscate(plainEnc)
	if err != nil {
		return ClientHello{}, err
	}

	var clientHello ClientHello
	unmarshalErr := clientHello.UnmarshalBinary(plain)
	if unmarshalErr != nil {
		return ClientHello{}, unmarshalErr
	}

	h.clientHelloObfuscated = obfuscated
	h.clientHelloEncrypted = encrypted
	return clientHello, nil
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

	if h.clientHelloObfuscated {
		marshalledServerHello, marshalErr = (Obfuscator{}).Obfuscate(marshalledServerHello)
		if marshalErr != nil {
			return marshalErr
		}
	}

	if h.clientHelloEncrypted {
		marshalledServerHello, marshalErr = h.enc.Encrypt(marshalledServerHello)
		if marshalErr != nil {
			return marshalErr
		}
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
