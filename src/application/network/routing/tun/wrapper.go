package tun

import "os"

type Wrapper interface {
	Wrap(f *os.File) (Device, error)
}
