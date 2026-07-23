package mtu

import "tungo/infrastructure/settings"

// Effective returns the MTU that can be applied to the configured IP stack.
func Effective(configuration settings.Settings) int {
	value := configuration.MTU
	if value <= 0 {
		value = settings.SafeMTU
	}

	minimum := settings.MinimumIPv4MTU
	if configuration.HasIPv6() {
		minimum = settings.MinimumIPv6MTU
	}
	return max(value, minimum)
}
