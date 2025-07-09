package hmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"tungo/application"
)

// CryptoHMAC - concurrently unsafe implementation of application.HMAC based on crypto/sha256 and crypto/hmac.
type CryptoHMAC struct {
	secret []byte
	// ioBuf is used to avoid memory allocations on Generate or Verify calls.
	// NOTE: each Generate or Verify call will rewrite ioBuf
	ioBuf [sha256.Size]byte
}

func NewHMAC(secret []byte) application.HMAC {
	return &CryptoHMAC{
		secret: secret,
	}
}

// Generate generates new HMAC data.
// NOTE: do not use it in concurrent environment as Generate is only valid before next Generate or Verify call.
func (d *CryptoHMAC) Generate(data []byte) ([]byte, error) {
	mac := hmac.New(sha256.New, d.secret)
	mac.Write(data)
	sum := mac.Sum(d.ioBuf[:0])
	return sum, nil
}

// Verify verifies HMAC data
// NOTE: do not use it in concurrent environment as Verify is only valid before next Generate or Verify call.
func (d *CryptoHMAC) Verify(data, signature []byte) error {
	mac := hmac.New(sha256.New, d.secret)
	mac.Write(data)
	expected := mac.Sum(d.ioBuf[:0])
	equal := hmac.Equal(expected, signature)
	if !equal {
		return ErrUnexpectedSignature
	}

	return nil
}
