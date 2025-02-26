package pipes

import (
	"io"
)

type (
	Pipe interface {
		Pass(data []byte) error
	}
	DefaultPipe struct {
		from io.Reader
		to   io.Writer
	}
)

func NewDefaultPipe(from io.Reader, to io.Writer) *DefaultPipe {
	return &DefaultPipe{
		from: from,
		to:   to,
	}
}

func (p *DefaultPipe) Pass(data []byte) error {
	_, err := p.to.Write(data)
	if err != nil {
		return err
	}

	return nil
}
