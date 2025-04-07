package mode

type Mode int

const (
	Unknown Mode = iota
	Client
	Server
)
