package runtime

type Mode uint8

const (
	ModeClient Mode = iota + 1
	ModeServer
)
