package iptables

import (
	"tungo/application"
	"tungo/infrastructure/PAL"
)

type Factory interface {
	New(v4bin, v6bin string) application.Netfilter
}

type DefaultFactory struct{ cmd PAL.Commander }

func NewDefaultFactory(cmd PAL.Commander) DefaultFactory {
	return DefaultFactory{cmd: cmd}
}

func (f DefaultFactory) New(v4bin, v6bin string) application.Netfilter {
	return NewDriverWithBinaries(f.cmd, v4bin, v6bin)
}
