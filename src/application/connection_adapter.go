package application

// ConnectionAdapter provides a single and trivial API for any supported transports
type ConnectionAdapter interface {
	Write([]byte) (int, error)
	Read([]byte) (int, error)
	Close() error
}
