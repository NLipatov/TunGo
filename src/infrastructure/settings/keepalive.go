package settings

import "time"

const (
	// PingInterval is how long the client waits without receiving any data
	// before sending a Ping to the server.
	PingInterval = 3 * time.Second

	// PingRestartTimeout is how long the client waits without receiving any
	// data before tearing down the session (server unreachable).
	PingRestartTimeout = 15 * time.Second
)
