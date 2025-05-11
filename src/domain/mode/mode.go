package mode

type Mode int

const (
	Unknown Mode = iota
	// Client mode used to start client
	Client
	// Server mode used to start server
	Server
	// ServerConfGen mode used to generate client configuration
	ServerConfGen
	// Version used to lookup version
	Version
)
