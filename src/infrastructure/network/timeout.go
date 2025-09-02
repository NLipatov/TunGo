package network

import (
	"errors"
	"time"
)

var ErrInvalidDuration = errors.New("invalid duration")

type Timeout time.Duration

func NewDeadline(d time.Duration) (Timeout, error) {
	if d < 0 {
		return 0, ErrInvalidDuration
	}
	return Timeout(d), nil
}

func (d Timeout) Time() time.Time {
	if d == 0 {
		return time.Time{}
	}
	return time.Now().Add(time.Duration(d))
}
