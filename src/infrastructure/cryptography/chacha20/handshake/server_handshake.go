package handshake

import (
	"crypto/ed25519"
	"fmt"
	"io"
	"tungo/application/network/connection"
)

type ServerHandshake struct {
	transport connection.Transport
}

func NewServerHandshake(transport connection.Transport) ServerHandshake {
	return ServerHandshake{
		transport: transport,
	}
}

func (h *ServerHandshake) ReceiveClientHello() (ClientHello, error) {
	buf := make([]byte, MaxClientHelloSizeBytes)

	var (
		totalRead  int
		targetSize = -1
	)

	for {
		if targetSize != -1 && totalRead >= targetSize {
			break
		}

		limit := MaxClientHelloSizeBytes - totalRead
		if limit == 0 {
			return ClientHello{}, fmt.Errorf("invalid Client Hello size: %d", MaxClientHelloSizeBytes)
		}
		if targetSize != -1 {
			if remaining := targetSize - totalRead; remaining < limit {
				limit = remaining
			}
		}

		n, err := h.transport.Read(buf[totalRead : totalRead+limit])
		if err != nil {
			if err == io.EOF && totalRead > 0 {
				err = io.ErrUnexpectedEOF
			}
			return ClientHello{}, err
		}
		if n == 0 {
			continue
		}

		totalRead += n

		if totalRead >= lengthHeaderLength && targetSize == -1 {
			ipLength := int(buf[1])
			targetSize = lengthHeaderLength + ipLength + curvePublicKeyLength + curvePublicKeyLength + nonceLength
			if targetSize > MaxClientHelloSizeBytes {
				return ClientHello{}, fmt.Errorf("invalid Client Hello size: %d", targetSize)
			}
		}

		if targetSize != -1 && totalRead > targetSize {
			return ClientHello{}, fmt.Errorf("invalid Client Hello size: %d", totalRead)
		}
	}

	if targetSize == -1 {
		return ClientHello{}, fmt.Errorf("failed to determine Client Hello size")
	}

	helloBuf := make([]byte, targetSize)
	copy(helloBuf, buf[:targetSize])

	hello := NewEmptyClientHelloWithDefaultIPValidator()
	if err := hello.UnmarshalBinary(helloBuf); err != nil {
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

	_, writeErr := h.transport.Write(marshalledServerHello)
	return writeErr
}

func (h *ServerHandshake) VerifyClientSignature(c Crypto, hello ClientHello, serverNonce []byte) error {
	clientSignatureBuf := make([]byte, 64)
	if _, err := io.ReadFull(h.transport, clientSignatureBuf); err != nil {
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
