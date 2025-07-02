package settings

import "time"

type SessionLifetime struct {
	Ttl             time.Duration `json:"Ttl"`
	CleanupInterval time.Duration `json:"CleanupInterval"`
}
