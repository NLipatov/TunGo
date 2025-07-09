package handshake

import "fmt"

type ServerHello struct {
	signature      []byte
	nonce          []byte
	curvePublicKey []byte
}

func NewServerHello(signature, nonce, pubKey []byte) ServerHello {
	return ServerHello{
		signature:      signature,
		nonce:          nonce,
		curvePublicKey: pubKey,
	}
}

func (h *ServerHello) Nonce() []byte {
	return h.nonce
}

func (h *ServerHello) CurvePublicKey() []byte {
	return h.curvePublicKey
}

func (h *ServerHello) MarshalBinary() ([]byte, error) {
	if len(h.signature) != signatureLength {
		return nil, fmt.Errorf("invalid signature")
	}
	if len(h.nonce) != nonceLength {
		return nil, InvalidNonce
	}
	if len(h.curvePublicKey) != curvePublicKeyLength {
		return nil, fmt.Errorf("invalid curve public key")
	}

	arr := make([]byte, signatureLength+nonceLength+curvePublicKeyLength)

	offset := 0
	copy(arr, h.signature)
	offset += signatureLength

	copy(arr[offset:], h.nonce)
	offset += nonceLength

	copy(arr[offset:], h.curvePublicKey)

	return arr, nil
}

func (h *ServerHello) UnmarshalBinary(data []byte) error {
	if len(data) != signatureLength+nonceLength+curvePublicKeyLength {
		return fmt.Errorf("invalid data")
	}

	offset := 0
	h.signature = data[:signatureLength]
	offset += signatureLength

	h.nonce = data[offset : offset+nonceLength]
	offset += nonceLength

	h.curvePublicKey = data[offset : offset+curvePublicKeyLength]
	return nil
}
