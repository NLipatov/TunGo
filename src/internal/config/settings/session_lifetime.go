package settings

import (
	"encoding/json"
	"time"
)

type SessionLifetime struct {
	Ttl             HumanReadableDuration `json:"Ttl"`
	CleanupInterval HumanReadableDuration `json:"CleanupInterval"`
}

type HumanReadableDuration time.Duration

func (d HumanReadableDuration) MarshalJSON() ([]byte, error) {
	s := time.Duration(d).String() // e.g., "45m" or "12h"
	return json.Marshal(s)
}

func (d *HumanReadableDuration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	duration, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = HumanReadableDuration(duration)
	return nil
}
