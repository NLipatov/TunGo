package routing

// Endpoints are the two directions of a tunnel. The protocol implementation
// owns both functions; Router only runs them concurrently.
type Endpoints struct {
	RunTun       func() error
	RunTransport func() error
}
