package mode

// Mode identifies which side of the tunnel the process runs.
type Mode uint8

const (
	Client Mode = iota + 1
	Server
)
