package session

import "errors"

var (
	ErrNotFound       = errors.New("session not found")
	ErrTransportWrite = errors.New("transport write failed")
	ErrEncryption     = errors.New("encryption failed")
)
