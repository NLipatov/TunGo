package client

import (
	"errors"
)

var (
	ErrNoFailoverCandidates = errors.New("no failover option available")
)

type Failover struct {
	configurations   *Configurations
	cur              int
	count, threshold int
}

func NewFailover(configurations *Configurations, failThreshold int) *Failover {
	return &Failover{
		configurations: configurations,
		threshold:      failThreshold,
	}
}

func (f *Failover) Reset() {
	f.count = 0
}

func (f *Failover) OnFail() (bool, string, error) {
	f.count++
	if f.count < f.threshold {
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
