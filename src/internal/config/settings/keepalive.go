package settings

import "time"

const (
	// PingInterval is how long the client waits without receiving any data
	// before sending a Ping to the server.
	PingInterval = 3 * time.Second

	// PingRestartTimeout is how long the client waits without receiving any
	// data before tearing down the session (server unreachable).
	PingRestartTimeout = 15 * time.Second

	// ServerIdleTimeout is how long the server waits without receiving any
	// data before closing a client session (client presumed dead).
	// Must be significantly larger than PingInterval to tolerate jitter.
	ServerIdleTimeout = 30 * time.Second

	// IdleReaperInterval is how often the server scans for idle sessions.
	IdleReaperInterval = 10 * time.Second
)
