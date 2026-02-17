package confgen

import (
	"fmt"
	"net"
	"net/netip"
	"tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/settings"
)

// hostResolver resolves the server's outbound IPv4 and IPv6 addresses.
type hostResolver interface {
	ResolveIPv4() (string, error)
	ResolveIPv6() (string, error)
}

// dialHostResolver uses net.Dial to discover the outbound source address
// the kernel would pick. No traffic is actually sent (UDP dial is local).
type dialHostResolver struct{}

func (dialHostResolver) ResolveIPv4() (string, error) {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

func (dialHostResolver) ResolveIPv6() (string, error) {
	conn, err := net.Dial("udp6", "[2001:4860:4860::8888]:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

type Generator struct {
	resolver                   hostResolver
	serverConfigurationManager serverConfiguration.ConfigurationManager
	keyDeriver                 primitives.KeyDeriver
}

func NewGenerator(
	serverConfigurationManager serverConfiguration.ConfigurationManager,
	keyDeriver primitives.KeyDeriver,
) *Generator {
	return &Generator{
		resolver:                   dialHostResolver{},
		serverConfigurationManager: serverConfigurationManager,
		keyDeriver:                 keyDeriver,
	}
}

// Generate creates a new client configuration, registers the peer with the server,
// and returns the resulting client.Configuration.
func (g *Generator) Generate() (*client.Configuration, error) {
	serverConf, err := g.serverConfigurationManager.Configuration()
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration: %w", err)
	}

	serverHost, err := g.resolveServerHost(serverConf.FallbackServerAddress)
	if err != nil {
		return nil, err
	}

	if serverHost.HasIPv6() {
		if err := g.serverConfigurationManager.EnsureIPv6Subnets(); err != nil {
			return nil, fmt.Errorf("failed to auto-enable IPv6 subnets: %w", err)
		}
		// Re-read config after subnets may have been written.
		serverConf, err = g.serverConfigurationManager.Configuration()
		if err != nil {
			return nil, fmt.Errorf("failed to re-read server configuration: %w", err)
		}
	}

	clientID := serverConf.ClientCounter + 1

	if err := g.serverConfigurationManager.IncrementClientCounter(); err != nil {
		return nil, err
	}

	clientPubKey, clientPrivKey, err := g.keyDeriver.GenerateX25519KeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate client keypair: %w", err)
	}

	newPeer := serverConfiguration.AllowedPeer{
		Name:      fmt.Sprintf("client-%d", clientID),
		PublicKey: clientPubKey,
		Enabled:   true,
		ClientID:  clientID,
	}
	if err := g.serverConfigurationManager.AddAllowedPeer(newPeer); err != nil {
		return nil, fmt.Errorf("failed to add client to AllowedPeers: %w", err)
	}

	defaultProtocol := getDefaultProtocol(serverConf)

	conf := client.Configuration{
		ClientID:         clientID,
		TCPSettings:      deriveClientSettings(serverConf.TCPSettings, serverHost, settings.TCP),
		UDPSettings:      deriveClientSettings(serverConf.UDPSettings, serverHost, settings.UDP),
		WSSettings:       deriveClientSettings(serverConf.WSSettings, serverHost, settings.WS),
		X25519PublicKey:  serverConf.X25519PublicKey,
		Protocol:         defaultProtocol,
		ClientPublicKey:  clientPubKey,
		ClientPrivateKey: clientPrivKey[:],
	}

	return &conf, nil
}

func (g *Generator) resolveServerHost(fallback string) (settings.Host, error) {
	ipv4Str, ipv4Err := g.resolver.ResolveIPv4()
	if ipv4Err != nil {
		if fallback == "" {
			return settings.Host{}, fmt.Errorf(
				"failed to resolve server IP and no fallback address provided in server configuration: %w",
				ipv4Err,
			)
		}
		ipv4Str = fallback
	}

	host, err := settings.NewHost(ipv4Str)
	if err != nil {
		return settings.Host{}, fmt.Errorf("invalid server host %q: %w", ipv4Str, err)
	}

	ipv6Str, ipv6Err := g.resolver.ResolveIPv6()
	if ipv6Err == nil {
		if ipv6Addr, parseErr := netip.ParseAddr(ipv6Str); parseErr == nil {
			host = host.WithIPv6(ipv6Addr)
		}
	}

	return host, nil
}

// deriveClientSettings copies subnets from server settings into a client Settings.
// IPv4/IPv6 addresses are NOT set here — they are derived at load time via Resolve().
func deriveClientSettings(
	serverSettings settings.Settings,
	serverHost settings.Host,
	protocol settings.Protocol,
) settings.Settings {
	mtu := serverSettings.MTU
	if protocol == settings.UDP {
		mtu = settings.SafeMTU
	}
	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    serverSettings.TunName,
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
	return s
}

func getDefaultProtocol(conf *serverConfiguration.Configuration) settings.Protocol {
	if conf.EnableUDP {
		return settings.UDP
	}
	if conf.EnableTCP {
		return settings.TCP
	}
	return settings.WS
}
