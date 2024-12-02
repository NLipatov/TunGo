package ChaCha20

import (
	"errors"
)

var ErrNonUniqueNonce = errors.New("critical decryption error: nonce was not unique")
