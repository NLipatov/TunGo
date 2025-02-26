package pipes

import (
	"io"
)

type ReaderPipe struct {
	pipe Pipe
	from io.Reader
}

func NewReaderPipe(pipe Pipe, from io.Reader) Pipe {
	return &ReaderPipe{
		pipe: pipe,
		from: from,
	}
}

func (rp *ReaderPipe) Pass(buffer []byte) error {
	n, readErr := rp.from.Read(buffer)
	if readErr != nil {
		return readErr
	}

	return rp.pipe.Pass(buffer[:n])
}
