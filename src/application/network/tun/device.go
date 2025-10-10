package tun

// Device provides a single and trivial API for any supported tun_device devices
type Device interface {
	Read(data []byte) (int, error)
	Write(data []byte) (int, error)
	Close() error
}
