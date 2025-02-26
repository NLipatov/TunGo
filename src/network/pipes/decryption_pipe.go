package pipes

import "tungo/crypto"

type DecryptionPipe struct {
	pipe    Pipe
	session crypto.Session
}

func NewDecryptionPipe(pipe Pipe, session crypto.Session) *DecryptionPipe {
	return &DecryptionPipe{
		pipe:    pipe,
		session: session,
	}
}

func (dp *DecryptionPipe) Pass(data []byte) error {
	decryptedData, decryptedDataErr := dp.session.Decrypt(data)
	if decryptedDataErr != nil {
		return decryptedDataErr
	}

	return dp.pipe.Pass(decryptedData)
}
