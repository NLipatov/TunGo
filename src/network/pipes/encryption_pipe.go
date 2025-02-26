package pipes

import (
	"tungo/crypto"
)

type EncryptionPipe struct {
	pipe    Pipe
	session crypto.Session
}

func NewEncryptionPipe(pipe Pipe, session crypto.Session) Pipe {
	return &EncryptionPipe{
		pipe:    pipe,
		session: session,
	}
}

func (sp *EncryptionPipe) Pass(data []byte) error {
	encryptedData, encryptedDataErr := sp.session.Encrypt(data)
	if encryptedDataErr != nil {
		return encryptedDataErr
	}

	return sp.pipe.Pass(encryptedData)
}
