//go:build darwin

package tun_adapters

type Factory interface {
	CreateTUN(mtu int) (Adapter, error)
}

// Adapter is your low-level vector I/O interface used by WgTunAdapter.
type Adapter interface {
	Read(frags [][]byte, sizes []int, offset int) (int, error)
	Write(frags [][]byte, offset int) (int, error)
	Close() error
	Name() (string, error)
}
