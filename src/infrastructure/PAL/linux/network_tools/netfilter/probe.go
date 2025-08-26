package netfilter

import nftlib "github.com/google/nftables"

type Probe interface {
	Supports() (bool, error)
}

// DefaultProbe is the production probe that talks to the kernel via netlink.
type DefaultProbe struct{}

func (DefaultProbe) Supports() (bool, error) {
	c, err := nftlib.New()
	if err != nil {
		return false, err
	}
	_ = c.CloseLasting() // safe no-op for non-lasting conns
	if _, err := c.ListTables(); err != nil {
		return false, err
	}
	return true, nil
}
