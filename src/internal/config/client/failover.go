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

func (f *Failover) Failover() error {
	candidates, err := f.configurations.List()
	if err != nil {
		return err
	}
	if len(candidates) < 2 {
		return ErrNoFailoverCandidates
	}
	err = f.configurations.Activate(candidates[f.cur%len(candidates)])
	f.cur++
	return err
}
