package chacha20

import (
	"errors"
)

var (
	ErrNonUniqueNonce   = errors.New("critical decryption error: nonce was not unique")
	ErrInvalidNonceSize = errors.New("critical decryption error: invalid nonce size")
)
