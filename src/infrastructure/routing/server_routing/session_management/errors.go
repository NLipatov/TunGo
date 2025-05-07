package session_management

import "errors"

var ErrSessionNotFound = errors.New("session not found")
var ErrInvalidIPLength = errors.New("invalid IP length")
