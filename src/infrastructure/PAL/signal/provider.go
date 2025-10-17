package signal

import "os"

// Provider abstracts platform-specific signals
type Provider interface {
	ShutdownSignals() []os.Signal
}
