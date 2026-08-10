package config

// Mode identifies which side of the tunnel the process runs.
type Mode uint8

const (
	ModeClient Mode = iota + 1
	ModeServer
)
