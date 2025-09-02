package network

import (
	"time"
)

type Deadline struct {
	expiresAt time.Time // zero deadline mean no deadline
}

func InfiniteDeadline() Deadline {
	return Deadline{
		expiresAt: time.Time{},
	}
}

func DeadlineFromTime(deadline time.Time) (Deadline, error) {
	return Deadline{
		expiresAt: deadline,
	}, nil
}

func (d Deadline) ExpiresAt() time.Time {
	return d.expiresAt
}
