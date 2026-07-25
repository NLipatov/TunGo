package ne

import (
	"fmt"

	"tungo/application/configuration"
	"tungo/infrastructure/network/mtu"
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

type Settings struct {
	RemoteAddress              string      `json:"remoteAddress"`
	MTU                        int         `json:"mtu"`
	StartupTimeoutMilliseconds int         `json:"startupTimeoutMilliseconds"`
	IPv4                       *IPSettings `json:"ipv4,omitempty"`
	IPv6                       *IPSettings `json:"ipv6,omitempty"`
	DNSServers                 []string    `json:"dnsServers,omitempty"`
	IncludedRoutes             []Route     `json:"includedRoutes"`
	ExcludedRoutes             []Route     `json:"excludedRoutes"`
}

// LoadSettings loads the active client configuration and translates it
// into the payload consumed by the Swift NetworkExtension adapter.
func LoadSettings() (Settings, error) {
	conf, err := configuration.NewDefaultClientControl().ClientRuntimeConfiguration()
	if err != nil {
		return Settings{}, err
	}
	return NewSettings(conf)
}

// NewSettings translates TunGo client settings into the payload
// consumed by the Swift NetworkExtension adapter.
func NewSettings(conf configuration.ClientRuntimeConfiguration) (Settings, error) {
	active, err := conf.ActiveSettings()
	if err != nil {
		return Settings{}, err
	}
	if active.Server.IsZero() {
		return Settings{}, fmt.Errorf("active settings: Server is not configured")
	}

	networkSettings := Settings{
		RemoteAddress:              active.Server.String(),
		MTU:                        mtu.Effective(active),
		StartupTimeoutMilliseconds: startupTimeoutMilliseconds(active),
		IncludedRoutes:             make([]Route, 0, 2),
		ExcludedRoutes:             make([]Route, 0),
	}
	if active.IPv4.IsValid() {
		networkSettings.IPv4 = &IPSettings{Address: active.IPv4.Unmap().String(), PrefixLength: 32}
		networkSettings.IncludedRoutes = append(
			networkSettings.IncludedRoutes,
			Route{Destination: "0.0.0.0", PrefixLength: 0},
		)
		networkSettings.DNSServers = append(networkSettings.DNSServers, active.DNSv4Resolvers()...)
	}
	if active.IPv6.IsValid() {
		prefixLength := 128
		if active.IPv6Subnet.IsValid() {
			prefixLength = active.IPv6Subnet.Bits()
		}
		networkSettings.IPv6 = &IPSettings{Address: active.IPv6.String(), PrefixLength: prefixLength}
		networkSettings.IncludedRoutes = append(
			networkSettings.IncludedRoutes,
			Route{Destination: "::", PrefixLength: 0},
		)
		networkSettings.DNSServers = append(networkSettings.DNSServers, active.DNSv6Resolvers()...)
	}
	if networkSettings.IPv4 == nil && networkSettings.IPv6 == nil {
		return Settings{}, fmt.Errorf("active settings: no resolved tunnel address")
	}
	return networkSettings, nil
}

func startupTimeoutMilliseconds(active settings.Settings) int {
	timeout := max(int(active.DialTimeoutMs), 5000)
	return timeout + 1000
}
