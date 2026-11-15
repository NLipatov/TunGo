package settings

import (
	"time"
)

const (
	DefaultEthernetMTU = 1500
	DefaultMTU         = 1280
	MinimumIPv4MTU     = 576
	MinimumIPv6MTU     = 1280
	MaximumMTU         = 9000
	DefaultDialTimeout = HumanReadableDuration(5000 * time.Millisecond)
)
