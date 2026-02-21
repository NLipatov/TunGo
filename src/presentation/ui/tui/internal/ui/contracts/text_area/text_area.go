package text_area

import "errors"

var ErrCancelled = errors.New("text area cancelled")

type TextArea interface {
	Value() (string, error)
}
