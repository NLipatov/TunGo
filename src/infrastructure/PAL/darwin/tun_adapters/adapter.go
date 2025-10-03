package tun_adapters

type Adapter interface {
	Read(buffer [][]byte, sizes []int, offset int) (n int, err error)
	Write(buffer [][]byte, offset int) (int, error)
	Close() error
	Name() (string, error)
}
