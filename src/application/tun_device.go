package application

// TunDevice provides a single and trivial API for any supported tun_device devices
type TunDevice interface {
	Read(data []byte) (int, error)
	Write(data []byte) (int, error)
	Close() error
}
