package handshake

import "fmt"

type ServerHello struct {
	Signature      []byte
	Nonce          []byte
	CurvePublicKey []byte
}

func NewServerHello(signature, nonce, pubKey []byte) ServerHello {
	return ServerHello{
		Signature:      signature,
		Nonce:          nonce,
		CurvePublicKey: pubKey,
	}
}

func (h *ServerHello) MarshalBinary() ([]byte, error) {
	if len(h.Signature) != signatureLength {
		return nil, fmt.Errorf("invalid signature")
	}
	if len(h.Nonce) != nonceLength {
		return nil, InvalidNonce
	}
	if len(h.CurvePublicKey) != curvePublicKeyLength {
		return nil, fmt.Errorf("invalid curve public key")
	}

	arr := make([]byte, signatureLength+nonceLength+curvePublicKeyLength)

	offset := 0
	copy(arr, h.Signature)
	offset += signatureLength

	copy(arr[offset:], h.Nonce)
	offset += nonceLength

	copy(arr[offset:], h.CurvePublicKey)

	return arr, nil
}

func (h *ServerHello) UnmarshalBinary(data []byte) error {
	if len(data) < signatureLength+nonceLength+curvePublicKeyLength {
		return fmt.Errorf("invalid data")
	}

	h.Signature = data[:signatureLength]
	h.Nonce = data[signatureLength : signatureLength+nonceLength]
	h.CurvePublicKey = data[signatureLength+nonceLength : signatureLength+nonceLength+curvePublicKeyLength]

	return nil
}
