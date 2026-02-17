package chacha20

import (
	"errors"
)

var ErrNonUniqueNonce = errors.New("critical decryption error: nonce was not unique")
var ErrUnknownEpoch = errors.New("unknown or expired epoch")
var ErrUnknownRouteID = errors.New("unknown route id")
