package stat

import "os"

type Stat interface {
	Stat(name string) (os.FileInfo, error)
}

type DefaultStat struct {
}

func NewDefaultStat() Stat {
	return &DefaultStat{}
}

func (d DefaultStat) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}
