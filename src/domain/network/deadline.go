package network

import (
	"errors"
	"time"
)

var (
	ErrDeadlineInPast = errors.New("deadline is in past")
)

type Deadline struct {
	expiresAt time.Time
}

func DeadlineFromTime(deadline time.Time) (Deadline, error) {
	// zero deadline means no deadline
	if deadline.IsZero() {
		return Deadline{
			expiresAt: deadline,
		}, nil
	}

	// deadline must not be in past
	now := time.Now()
	if !deadline.After(now) {
		return Deadline{}, ErrDeadlineInPast
	}

	return Deadline{
		expiresAt: deadline,
	}, nil
}

func (d Deadline) ExpiresAt() time.Time {
	return d.expiresAt
}
