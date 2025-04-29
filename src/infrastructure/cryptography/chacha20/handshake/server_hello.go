package handshake

import "fmt"

type ServerHello struct {
	Signature      []byte
	Nonce          []byte
	CurvePublicKey []byte
}

func (s *ServerHello) Read(data []byte) (*ServerHello, error) {
	if len(data) < signatureLength+nonceLength+curvePublicKeyLength {
		return nil, fmt.Errorf("invalid data")
	}

	s.Signature = data[:signatureLength]
	s.Nonce = data[signatureLength : signatureLength+nonceLength]
	s.CurvePublicKey = data[signatureLength+nonceLength : signatureLength+nonceLength+curvePublicKeyLength]

	return s, nil
}

func (s *ServerHello) Write(signature *[]byte, nonce *[]byte, curvePublicKey *[]byte) (*[]byte, error) {
	if len(*signature) != signatureLength {
		return nil, fmt.Errorf("invalid signature")
	}
	if len(*nonce) != nonceLength {
		return nil, InvalidNonce
	}
	if len(*curvePublicKey) != curvePublicKeyLength {
		return nil, fmt.Errorf("invalid curve public key")
	}

	arr := make([]byte, signatureLength+nonceLength+curvePublicKeyLength)

	copy(arr, *signature)
	copy(arr[len(*signature):], *nonce)
	copy(arr[len(*signature)+len(*nonce):], *curvePublicKey)

	return &arr, nil
}
