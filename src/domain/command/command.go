package command

type Command int

const (
	Unknown Command = iota
	// StartClient starts the client tunnel.
	StartClient
	// StartServer starts the server tunnel.
	StartServer
	// GenerateClientConfig prints a client configuration derived from the server configuration.
	GenerateClientConfig
	// ShowVersion prints the application version.
	ShowVersion
)
