package apple

import (
	"fmt"

	"tungo/application/configuration"
	"tungo/infrastructure/settings"
)

type IPSettings struct {
	Address      string `json:"address"`
	PrefixLength int    `json:"prefixLength"`
}

type Route struct {
	Destination  string `json:"destination"`
	PrefixLength int    `json:"prefixLength"`
}

type TunnelPlan struct {
	RemoteAddress              string      `json:"remoteAddress"`
	MTU                        int         `json:"mtu"`
	StartupTimeoutMilliseconds int         `json:"startupTimeoutMilliseconds"`
	IPv4                       *IPSettings `json:"ipv4,omitempty"`
	IPv6                       *IPSettings `json:"ipv6,omitempty"`
	DNSServers                 []string    `json:"dnsServers,omitempty"`
	IncludedRoutes             []Route     `json:"includedRoutes"`
	ExcludedRoutes             []Route     `json:"excludedRoutes"`
}

// NewTunnelPlan translates TunGo client settings into the platform-neutral
// description consumed by the Swift NetworkExtension adapter.
func NewTunnelPlan(conf configuration.ClientRuntimeConfiguration) (TunnelPlan, error) {
	active, err := conf.ActiveSettings()
	if err != nil {
		return TunnelPlan{}, err
	}
	if active.Server.IsZero() {
		return TunnelPlan{}, fmt.Errorf("active settings: Server is not configured")
	}

	plan := TunnelPlan{
		RemoteAddress:              active.Server.String(),
		MTU:                        effectiveMTU(active),
		StartupTimeoutMilliseconds: startupTimeoutMilliseconds(active),
		IncludedRoutes:             make([]Route, 0, 2),
		ExcludedRoutes:             make([]Route, 0),
	}
	if active.IPv4.IsValid() {
		plan.IPv4 = &IPSettings{Address: active.IPv4.Unmap().String(), PrefixLength: 32}
		plan.IncludedRoutes = append(plan.IncludedRoutes, Route{Destination: "0.0.0.0", PrefixLength: 0})
		plan.DNSServers = append(plan.DNSServers, active.DNSv4Resolvers()...)
	}
	if active.IPv6.IsValid() {
		prefixLength := 128
		if active.IPv6Subnet.IsValid() {
			prefixLength = active.IPv6Subnet.Bits()
		}
		plan.IPv6 = &IPSettings{Address: active.IPv6.String(), PrefixLength: prefixLength}
		plan.IncludedRoutes = append(plan.IncludedRoutes, Route{Destination: "::", PrefixLength: 0})
		plan.DNSServers = append(plan.DNSServers, active.DNSv6Resolvers()...)
	}
	if plan.IPv4 == nil && plan.IPv6 == nil {
		return TunnelPlan{}, fmt.Errorf("active settings: no resolved tunnel address")
	}
	return plan, nil
}

func startupTimeoutMilliseconds(active settings.Settings) int {
	timeout := int(active.DialTimeoutMs)
	if timeout < 5000 {
		timeout = 5000
	}
	return timeout + 1000
}

func effectiveMTU(active settings.Settings) int {
	mtu := active.MTU
	if mtu <= 0 {
		mtu = settings.SafeMTU
	}
	minimum := settings.MinimumIPv4MTU
	if active.IPv6.IsValid() {
		minimum = settings.MinimumIPv6MTU
	}
	if mtu < minimum {
		mtu = minimum
	}
	return mtu
}
