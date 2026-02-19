package text_input

import "errors"

var ErrCancelled = errors.New("text input cancelled")

type TextInput interface {
	Value() (string, error)
}
