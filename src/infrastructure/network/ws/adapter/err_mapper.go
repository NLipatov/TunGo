package adapter

import (
	"errors"
	"io"

	"github.com/coder/websocket"
)

// ErrorMapper normalizes transport errors to net.Conn semantics.
type ErrorMapper interface {
	Map(err error) error
}

type DefaultErrorMapper struct{}

func (DefaultErrorMapper) Map(err error) error {
	if err == nil {
		return nil
	}
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		switch ce.Code {
		case websocket.StatusNormalClosure, websocket.StatusGoingAway:
			return io.EOF
		}
	}
	return err
}
