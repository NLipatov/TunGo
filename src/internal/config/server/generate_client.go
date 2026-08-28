package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/protocol/keys"
	"tungo/internal/transport/host"
)

const (
	clientTCPTunName = "c_tcptun0"
	clientUDPTunName = "c_udptun0"
	clientWSTunName  = "c_wstun0"
)

type GeneratedClient struct {
	JSON string
	Path string
}

// GenerateClient creates a client configuration and registers its peer.
func (f *File) GenerateClient() (GeneratedClient, error) {
	if err := f.EnsureKeys(); err != nil {
		return GeneratedClient{}, fmt.Errorf("could not prepare server keys: %w", err)
	}
	serverConfiguration, err := f.Load()
	if err != nil {
		return GeneratedClient{}, fmt.Errorf("failed to read server configuration: %w", err)
	}
	serverHost, err := resolveServerHost(serverConfiguration.Host)
	if err != nil {
		return GeneratedClient{}, err
	}
	ipv6SubnetsAdded := false
	if serverHost.IPv6 != "" {
		ensureIPv6Subnets(serverConfiguration)
	}

	clientID := serverConfiguration.ClientCounter + 1
	publicKey, privateKey, err := (&keys.DefaultKeyDeriver{}).GenerateX25519KeyPair()
	if err != nil {
		return GeneratedClient{}, fmt.Errorf("failed to generate client keypair: %w", err)
	}
	defer clear(privateKey[:])

	tcpSettings, err := deriveClientSettings(serverConfiguration.TCPSettings, serverHost, settings.TCP)
	if err != nil {
		return GeneratedClient{}, fmt.Errorf("failed to derive tcp settings: %w", err)
	}
	udpSettings, err := deriveClientSettings(serverConfiguration.UDPSettings, serverHost, settings.UDP)
	if err != nil {
		return GeneratedClient{}, fmt.Errorf("failed to derive udp settings: %w", err)
	}
	wsSettings, err := deriveClientSettings(serverConfiguration.WSSettings, serverHost, settings.WS)
	if err != nil {
		return GeneratedClient{}, fmt.Errorf("failed to derive ws settings: %w", err)
	}
	configuration := client.Configuration{
		ClientID:         clientID,
		TCPSettings:      tcpSettings,
		UDPSettings:      udpSettings,
		WSSettings:       wsSettings,
		X25519PublicKey:  append([]byte(nil), serverConfiguration.X25519PublicKey...),
		Protocol:         defaultProtocol(serverConfiguration),
		ClientPublicKey:  append([]byte(nil), publicKey...),
		ClientPrivateKey: append([]byte(nil), privateKey[:]...),
	}
	data, err := json.MarshalIndent(configuration, "", "  ")
	if err != nil {
		return GeneratedClient{}, fmt.Errorf("failed to marshal client configuration: %w", err)
	}
	path := filepath.Join(filepath.Dir(f.path), fmt.Sprintf("client_configuration.json.%d", clientID))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return GeneratedClient{}, fmt.Errorf("failed to create server config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return GeneratedClient{}, fmt.Errorf("failed to save client configuration: %w", err)
	}

	serverConfiguration.ClientCounter = clientID
	serverConfiguration.AllowedPeers = append(serverConfiguration.AllowedPeers, AllowedPeer{
		Name:      fmt.Sprintf("client-%d", clientID),
		PublicKey: append([]byte(nil), publicKey...),
		Enabled:   true,
		ClientID:  clientID,
	})
	if err := f.write(*serverConfiguration); err != nil {
		registrationErr := fmt.Errorf("failed to register client: %w", err)
		if removeErr := os.Remove(path); removeErr != nil {
			return GeneratedClient{}, errors.Join(
				registrationErr,
				fmt.Errorf("failed to remove unregistered client configuration %q: %w", path, removeErr),
			)
		}
		return GeneratedClient{}, registrationErr
	}
	if ipv6SubnetsAdded {
		slog.Info("server IPv6 tunnel subnets added", "path", f.path)
	}
	return GeneratedClient{JSON: string(data), Path: path}, nil
}

// ensureIPv6Subnets assigns default IPv6 subnets to protocols without valid subnet configuration.
func ensureIPv6Subnets(configuration *Configuration) bool {
	defaults := [...]netip.Prefix{
		netip.MustParsePrefix("fd00::/64"),
		netip.MustParsePrefix("fd00:1::/64"),
		netip.MustParsePrefix("fd00:2::/64"),
	}
	added := false
	for i, protocolSettings := range configuration.settings() {
		if !protocolSettings.IPv6Subnet.IsValid() {
			protocolSettings.IPv6Subnet = defaults[i]
			added = true
		}
	}
	configuration.applyDefaults()
	return added
}

// resolveServerHost resolves a configured server host or detects available IPv4 and IPv6 addresses.
// It returns an error when no valid server host can be resolved.
func resolveServerHost(configured string) (settings.Host, error) {
	if configured != "" {
		value := strings.TrimSpace(configured)
		if ip, err := netip.ParseAddr(strings.Trim(value, "[]")); err == nil {
			ip = ip.Unmap()
			if ip.Is4() {
				return settings.Host{IPv4: ip.String()}, nil
			}
			return settings.Host{IPv6: ip.String()}, nil
		}
		return settings.Host{Domain: value}, nil
	}

	resolver := host.NewDialResolver()
	var resolved settings.Host
	ipv4String, ipv4Err := resolver.ResolveIPv4()
	if ipv4Err == nil {
		ipv4, err := netip.ParseAddr(ipv4String)
		if err != nil || !ipv4.Unmap().Is4() {
			ipv4Err = fmt.Errorf("invalid detected IPv4 %q", ipv4String)
		} else {
			resolved.IPv4 = ipv4.Unmap().String()
		}
	}
	ipv6String, ipv6Err := resolver.ResolveIPv6()
	if ipv6Err == nil {
		ipv6, err := netip.ParseAddr(ipv6String)
		if err != nil || ipv6.Unmap().Is4() {
			ipv6Err = fmt.Errorf("invalid detected IPv6 %q", ipv6String)
		} else {
			resolved.IPv6 = ipv6.String()
		}
	}
	if resolved == (settings.Host{}) {
		return settings.Host{}, fmt.Errorf("failed to detect server host: IPv4: %v; IPv6: %v", ipv4Err, ipv6Err)
	}
	return resolved, nil
}

// deriveClientSettings builds client settings from the server settings, host, and protocol.
// UDP uses the default MTU. It returns an error for unsupported protocols.
func deriveClientSettings(
	serverSettings settings.Settings,
	serverHost settings.Host,
	protocol settings.Protocol,
) (settings.Settings, error) {
	mtu := serverSettings.MTU
	if protocol == settings.UDP {
		mtu = settings.DefaultMTU
	}
	tunName, err := clientTunName(protocol)
	if err != nil {
		return settings.Settings{}, err
	}
	clientSettings := settings.Settings{
		Network: settings.Network{
			TunName:    tunName,
			IPv4Subnet: serverSettings.IPv4Subnet,
			Server:     serverHost,
			Port:       serverSettings.Port,
		},
		MTU:           mtu,
		Protocol:      protocol,
		Encryption:    serverSettings.Encryption,
		DialTimeoutMs: serverSettings.DialTimeoutMs,
	}
	if serverSettings.IPv6Subnet.IsValid() {
		clientSettings.IPv6Subnet = serverSettings.IPv6Subnet
	}
	return clientSettings, nil
}

// clientTunName returns the client tunnel name for the specified protocol.
// It returns an error for unsupported protocols.
func clientTunName(protocol settings.Protocol) (string, error) {
	switch protocol {
	case settings.UDP:
		return clientUDPTunName, nil
	case settings.TCP:
		return clientTCPTunName, nil
	case settings.WS, settings.WSS:
		return clientWSTunName, nil
	default:
		return "", fmt.Errorf("unsupported protocol %q", protocol)
	}
}

// defaultProtocol selects the first enabled protocol in UDP, TCP, and WebSocket order.
func defaultProtocol(configuration *Configuration) settings.Protocol {
	if configuration.EnableUDP {
		return settings.UDP
	}
	if configuration.EnableTCP {
		return settings.TCP
	}
	return settings.WS
}
