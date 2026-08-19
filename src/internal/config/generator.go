package config

import (
	"fmt"
	"net/netip"
	"strings"
	"tungo/internal/config/client"
	serverconfig "tungo/internal/config/server"
	"tungo/internal/config/settings"
	"tungo/internal/protocol/keys"
)

// hostResolver resolves the server's outbound IPv4 and IPv6 addresses.
type hostResolver interface {
	ResolveIPv4() (string, error)
	ResolveIPv6() (string, error)
}

type clientConfigGeneratorStore interface {
	Configuration() (*serverconfig.Configuration, error)
	IncrementClientCounter() error
	AddAllowedPeer(peer serverconfig.AllowedPeer) error
	EnsureIPv6Subnets() error
}

type generator struct {
	resolver            hostResolver
	serverconfigManager clientConfigGeneratorStore
	keyDeriver          keys.KeyDeriver
}

func newGenerator(
	serverconfigManager clientConfigGeneratorStore,
	keyDeriver keys.KeyDeriver,
	resolver hostResolver,
) *generator {
	return &generator{
		resolver:            resolver,
		serverconfigManager: serverconfigManager,
		keyDeriver:          keyDeriver,
	}
}

// Generate creates a new client configuration, registers the peer with the server,
// and returns the resulting client.Configuration.
func (g *generator) generate() (*client.Configuration, error) {
	serverConf, err := g.serverconfigManager.Configuration()
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration: %w", err)
	}

	serverHost, err := g.resolveServerHost(serverConf.Host)
	if err != nil {
		return nil, err
	}

	if serverHost.IPv6 != "" {
		if err := g.serverconfigManager.EnsureIPv6Subnets(); err != nil {
			return nil, fmt.Errorf("failed to auto-enable IPv6 subnets: %w", err)
		}
		// Re-read config after subnets may have been written.
		serverConf, err = g.serverconfigManager.Configuration()
		if err != nil {
			return nil, fmt.Errorf("failed to re-read server configuration: %w", err)
		}
	}

	clientID := serverConf.ClientCounter + 1

	if err := g.serverconfigManager.IncrementClientCounter(); err != nil {
		return nil, err
	}

	clientPubKey, clientPrivKey, err := g.keyDeriver.GenerateX25519KeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate client keypair: %w", err)
	}

	newPeer := serverconfig.AllowedPeer{
		Name:      fmt.Sprintf("client-%d", clientID),
		PublicKey: clientPubKey,
		Enabled:   true,
		ClientID:  clientID,
	}
	if err := g.serverconfigManager.AddAllowedPeer(newPeer); err != nil {
		return nil, fmt.Errorf("failed to add client to AllowedPeers: %w", err)
	}

	defaultProtocol := getDefaultProtocol(serverConf)
	tcpSettings, tcpErr := deriveClientSettings(serverConf.TCPSettings, serverHost, settings.TCP)
	if tcpErr != nil {
		return nil, fmt.Errorf("failed to derive tcp settings: %w", tcpErr)
	}
	udpSettings, udpErr := deriveClientSettings(serverConf.UDPSettings, serverHost, settings.UDP)
	if udpErr != nil {
		return nil, fmt.Errorf("failed to derive udp settings: %w", udpErr)
	}
	wsSettings, wsErr := deriveClientSettings(serverConf.WSSettings, serverHost, settings.WS)
	if wsErr != nil {
		return nil, fmt.Errorf("failed to derive ws settings: %w", wsErr)
	}
	conf := client.Configuration{
		ClientID:         clientID,
		TCPSettings:      tcpSettings,
		UDPSettings:      udpSettings,
		WSSettings:       wsSettings,
		X25519PublicKey:  serverConf.X25519PublicKey,
		Protocol:         defaultProtocol,
		ClientPublicKey:  clientPubKey,
		ClientPrivateKey: clientPrivKey[:],
	}

	return &conf, nil
}

func (g *generator) resolveServerHost(configured string) (settings.Host, error) {
	if configured != "" {
		value := strings.TrimSpace(configured)
		if ip, parseErr := netip.ParseAddr(strings.Trim(value, "[]")); parseErr == nil {
			ip = ip.Unmap()
			if ip.Is4() {
				return settings.Host{IPv4: ip.String()}, nil
			}
			return settings.Host{IPv6: ip.String()}, nil
		}
		return settings.Host{Domain: value}, nil
	}

	var host settings.Host
	ipv4Str, ipv4Err := g.resolver.ResolveIPv4()
	if ipv4Err == nil {
		ipv4, parseErr := netip.ParseAddr(ipv4Str)
		if parseErr != nil || !ipv4.Unmap().Is4() {
			ipv4Err = fmt.Errorf("invalid detected IPv4 %q", ipv4Str)
		} else {
			host.IPv4 = ipv4.Unmap().String()
		}
	}

	ipv6Str, ipv6Err := g.resolver.ResolveIPv6()
	if ipv6Err == nil {
		ipv6, parseErr := netip.ParseAddr(ipv6Str)
		if parseErr != nil || ipv6.Unmap().Is4() {
			ipv6Err = fmt.Errorf("invalid detected IPv6 %q", ipv6Str)
		} else {
			host.IPv6 = ipv6.String()
		}
	}

	if host == (settings.Host{}) {
		return settings.Host{}, fmt.Errorf(
			"failed to detect server host: IPv4: %v; IPv6: %v",
			ipv4Err,
			ipv6Err,
		)
	}

	return host, nil
}

// deriveClientSettings copies subnets from server settings into a client Settings.
// IPv4/IPv6 addresses are NOT set here — they are derived at load time via Resolve().
func deriveClientSettings(
	serverSettings settings.Settings,
	serverHost settings.Host,
	protocol settings.Protocol,
) (settings.Settings, error) {
	mtu := serverSettings.MTU
	if protocol == settings.UDP {
		mtu = settings.DefaultIPv4MTU
		if serverSettings.IPv6Subnet.IsValid() && !serverSettings.IPv6Subnet.Addr().Is4() {
			mtu = settings.DefaultIPv6MTU
		}
	}
	tunName, err := deriveClientTunName(protocol)
	if err != nil {
		return settings.Settings{}, err
	}
	s := settings.Settings{
		Addressing: settings.Addressing{
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
		s.IPv6Subnet = serverSettings.IPv6Subnet
	}
	return s, nil
}

func deriveClientTunName(protocol settings.Protocol) (string, error) {
	switch protocol {
	case settings.UDP:
		return clientUDPTunName, nil
	case settings.TCP:
		return clientTCPTunName, nil
	case settings.WS, settings.WSS:
		return clientWSTunName, nil
	default:
		return "", ErrUnsupportedProtocol
	}
}

func getDefaultProtocol(conf *serverconfig.Configuration) settings.Protocol {
	if conf.EnableUDP {
		return settings.UDP
	}
	if conf.EnableTCP {
		return settings.TCP
	}
	return settings.WS
}
