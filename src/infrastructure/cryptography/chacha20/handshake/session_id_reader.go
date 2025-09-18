package handshake

import (
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/hkdf"
)

type SessionIdReaderFactory interface {
	NewReader() io.Reader
}

type DefaultSessionIdReaderFactory struct {
	info, secret, salt []byte
}

func NewDefaultSessionIdReader(info, secret, salt []byte) DefaultSessionIdReaderFactory {
	return DefaultSessionIdReaderFactory{
		info:   info,
		secret: secret,
		salt:   salt,
	}
}

func (f DefaultSessionIdReaderFactory) NewReader() io.Reader {
	return hkdf.New(sha256.New, f.secret, f.salt, f.info)
}
