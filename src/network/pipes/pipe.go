package pipes

import (
	"io"
)

type (
	Pipe interface {
		Pass(data []byte) error
	}
	DefaultPipe struct {
		to io.Writer
	}
)

func NewDefaultPipe(to io.Writer) *DefaultPipe {
	return &DefaultPipe{
		to: to,
	}
}

func (p *DefaultPipe) Pass(data []byte) error {
	_, err := p.to.Write(data)
	if err != nil {
		return err
	}

	return nil
}
