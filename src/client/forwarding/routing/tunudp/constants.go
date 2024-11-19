package tunudp

import (
	"time"
)

const (
	initialBackoff       = 1 * time.Second
	maxBackoff           = 32 * time.Second
	maxReconnectAttempts = 30
)
