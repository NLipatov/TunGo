package routerImpl

import (
	"time"
)

const (
	initialBackoff       = 1 * time.Second
	maxBackoff           = 32 * time.Second
	maxReconnectAttempts = 5
	connectionTimeout    = 10 * time.Second
)
