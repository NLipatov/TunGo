//go:build windows

package manager

import (
	"time"
	"tungo/internal/config/settings"
)

func routeResolveTimeout(s settings.Settings) time.Duration {
	timeout := time.Duration(s.DialTimeoutMs) * time.Millisecond
	if timeout <= 0 {
		return 5 * time.Second
	}
	if timeout < 2*time.Second {
		return 2 * time.Second
	}
	return timeout
}
