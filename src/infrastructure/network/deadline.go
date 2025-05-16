package network

import (
	"errors"
	"time"
)

var ErrInvalidDuration = errors.New("invalid duration")

type Deadline time.Duration

func NewDeadline(d time.Duration) (Deadline, error) {
	if d < 0 {
		return 0, ErrInvalidDuration
	}
	return Deadline(d), nil
}

func (d Deadline) Time() time.Time {
	if d == 0 {
		return time.Time{}
	}
	return time.Now().Add(time.Duration(d))
}
