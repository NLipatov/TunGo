package boundary

type TunAdapter interface {
	Read(data []byte) (int, error)
	Write(data []byte) (int, error)
	Close() error
}
