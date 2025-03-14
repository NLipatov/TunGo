package chacha20

import (
	"errors"
)

var ErrNonUniqueNonce = errors.New("critical decryption error: nonce was not unique")
var InvalidNonce = errors.New("invalid nonce")
