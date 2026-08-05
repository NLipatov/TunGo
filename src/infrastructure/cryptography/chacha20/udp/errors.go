package udp

import (
	"errors"
	"tungo/infrastructure/cryptography/chacha20/internal/core"
)

var ErrNonUniqueNonce = errors.New("critical decryption error: nonce was not unique")
var ErrUnknownEpoch = core.ErrUnknownEpoch
var ErrUnknownRouteID = errors.New("unknown route id")
