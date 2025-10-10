package connection

// Transport provides a single and trivial API for any supported transports
type Transport interface {
	Write([]byte) (int, error)
	Read([]byte) (int, error)
	Close() error
}
