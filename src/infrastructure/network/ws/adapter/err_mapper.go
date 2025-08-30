package adapter

import (
	"errors"
	"io"

	"github.com/coder/websocket"
)

// errorMapper normalizes transport errors to net.Conn semantics.
type errorMapper interface {
	mapErr(err error) error
}

type defaultErrorMapper struct{}

func (defaultErrorMapper) mapErr(err error) error {
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
