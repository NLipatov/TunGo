package chacha20

import "fmt"

const (
	signatureLength      = 64
	nonceLength          = 32
	curvePublicKeyLength = 32
)

type ServerHello struct {
	Signature      []byte
	Nonce          []byte
	CurvePublicKey []byte
}

func (sH *ServerHello) Read(data []byte) (*ServerHello, error) {
	if len(data) < signatureLength+nonceLength+curvePublicKeyLength {
		return nil, fmt.Errorf("invalid data")
	}

	sH.Signature = data[:signatureLength]
	sH.Nonce = data[signatureLength : signatureLength+nonceLength]
	sH.CurvePublicKey = data[signatureLength+nonceLength : signatureLength+nonceLength+curvePublicKeyLength]

	return sH, nil
}

func (sH *ServerHello) Write(signature *[]byte, nonce *[]byte, curvePublicKey *[]byte) (*[]byte, error) {
	if len(*signature) != signatureLength {
		return nil, fmt.Errorf("invalid signature")
	}
	if len(*nonce) != nonceLength {
		return nil, fmt.Errorf("invalid nonce")
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
