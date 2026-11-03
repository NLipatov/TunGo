package client

import (
	"errors"
	"tungo/internal/config/client"
)

var (
	ErrNoFailoverCandidates = errors.New("no failover option available")
)

const failureThreshold = 3

type failover struct {
	configurations *client.Configurations
	cur            int
	count          int
}

func newFailover(configurations *client.Configurations) *failover {
	return &failover{
		configurations: configurations,
	}
}

func (f *failover) Reset() {
	f.count = 0
}

func (f *failover) OnFail() (bool, string, error) {
	f.count++
	if f.count < failureThreshold {
		return false, "", nil
	}
	candidates, err := f.configurations.List()
	if err != nil {
		return false, "", err
	}
	if len(candidates) < 2 {
		return false, "", ErrNoFailoverCandidates
	}
	candidate := candidates[f.cur%len(candidates)]
	f.cur++
	if err = f.configurations.Activate(candidate); err != nil {
		return false, candidate, err
	}
	f.count = 0
	return true, candidate, err
}
