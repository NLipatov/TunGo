package client

import (
	"errors"
)

var (
	ErrNoFailoverCandidates = errors.New("no failover option available")
)

type Failover struct {
	configurations *Configurations
	cur            int
}

func NewFailover(configurations *Configurations) Failover {
	return Failover{
		configurations: configurations,
	}
}

func (f *Failover) Failover() (string, error) {
	candidates, err := f.configurations.List()
	if err != nil {
		return "", err
	}
	if len(candidates) < 2 {
		return "", ErrNoFailoverCandidates
	}
	candidate := candidates[f.cur%len(candidates)]
	err = f.configurations.Activate(candidate)
	f.cur++
	return candidate, err
}
