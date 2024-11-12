package routing

import (
	"time"
)

const (
	initialBackoff       = 1 * time.Second
	maxBackoff           = 32 * time.Second
	maxReconnectAttempts = 30
	connectionTimeout    = 10 * time.Second
)
