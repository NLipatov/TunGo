package pipes

import (
	"io"
)

type ReaderPipe struct {
	pipe   Pipe
	reader io.Reader
}

func NewReaderPipe(pipe Pipe, reader io.Reader) Pipe {
	return &ReaderPipe{
		pipe:   pipe,
		reader: reader,
	}
}

func (rp *ReaderPipe) Pass(buffer []byte) error {
	n, readErr := rp.reader.Read(buffer)
	if readErr != nil {
		return readErr
	}

	return rp.pipe.Pass(buffer[:n])
}
