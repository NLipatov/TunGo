package settings

import "time"

// DefaultRekeyInterval defines how often key rotation is triggered by default
// for all transports (UDP, TCP, WS). Keep consistent across handlers.
const DefaultRekeyInterval = 5 * time.Minute
