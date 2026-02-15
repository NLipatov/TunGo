package confgen

import (
	"fmt"
	"net"
	"net/netip"
	"tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/primitives"
	nip "tungo/infrastructure/network/ip"
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

	clientIPs, err := g.allocateClientIPs(serverConf, clientID)
	if err != nil {
		return nil, err
	}

	clientPubKey, clientPrivKey, err := g.keyDeriver.GenerateX25519KeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate client keypair: %w", err)
	}

	newPeer := serverConfiguration.AllowedPeer{
		Name:        fmt.Sprintf("client-%d", clientID),
		PublicKey:   clientPubKey,
		Enabled:     true,
		ClientID: clientID,
	}
	if err := g.serverConfigurationManager.AddAllowedPeer(newPeer); err != nil {
		return nil, fmt.Errorf("failed to add client to AllowedPeers: %w", err)
	}

	defaultProtocol := getDefaultProtocol(serverConf)

	conf := client.Configuration{
		TCPSettings:      deriveClientSettings(serverConf.TCPSettings, clientIPs.tcp, clientIPs.tcpV6, serverHost, settings.TCP),
		UDPSettings:      deriveClientSettings(serverConf.UDPSettings, clientIPs.udp, clientIPs.udpV6, serverHost, settings.UDP),
		WSSettings:       deriveClientSettings(serverConf.WSSettings, clientIPs.ws, clientIPs.wsV6, serverHost, settings.WS),
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

type clientIPs struct {
	tcp, udp, ws       netip.Addr
	tcpV6, udpV6, wsV6 netip.Addr
}

func (g *Generator) allocateClientIPs(
	serverConf *serverConfiguration.Configuration,
	clientID int,
) (clientIPs, error) {
	var ips clientIPs
	var err error

	ips.tcp, err = nip.AllocateClientIP(serverConf.TCPSettings.IPv4Subnet, clientID)
	if err != nil {
		return clientIPs{}, fmt.Errorf("TCP interface address allocation fail: %w", err)
	}

	ips.udp, err = nip.AllocateClientIP(serverConf.UDPSettings.IPv4Subnet, clientID)
	if err != nil {
		return clientIPs{}, fmt.Errorf("UDP interface address allocation fail: %w", err)
	}

	ips.ws, err = nip.AllocateClientIP(serverConf.WSSettings.IPv4Subnet, clientID)
	if err != nil {
		return clientIPs{}, fmt.Errorf("WS interface address allocation fail: %w", err)
	}

	// Allocate IPv6 addresses (optional — only if server has IPv6 subnets configured)
	if serverConf.TCPSettings.IPv6Subnet.IsValid() {
		ips.tcpV6, _ = nip.AllocateClientIP(serverConf.TCPSettings.IPv6Subnet, clientID)
	}
	if serverConf.UDPSettings.IPv6Subnet.IsValid() {
		ips.udpV6, _ = nip.AllocateClientIP(serverConf.UDPSettings.IPv6Subnet, clientID)
	}
	if serverConf.WSSettings.IPv6Subnet.IsValid() {
		ips.wsV6, _ = nip.AllocateClientIP(serverConf.WSSettings.IPv6Subnet, clientID)
	}

	if err = g.serverConfigurationManager.IncrementClientCounter(); err != nil {
		return clientIPs{}, err
	}

	return ips, nil
}

func deriveClientSettings(
	serverSettings settings.Settings,
	clientIP netip.Addr,
	clientIPv6 netip.Addr,
	serverHost settings.Host,
	protocol settings.Protocol,
) settings.Settings {
	mtu := serverSettings.MTU
	if protocol == settings.UDP {
		mtu = settings.SafeMTU
	}
	s := settings.Settings{
		InterfaceName: serverSettings.InterfaceName,
		IPv4Subnet:    serverSettings.IPv4Subnet,
		IPv4IP:        clientIP.Unmap(),
		Host:          serverHost,
		Port:          serverSettings.Port,
		MTU:           mtu,
		Protocol:      protocol,
		Encryption:    serverSettings.Encryption,
		DialTimeoutMs: serverSettings.DialTimeoutMs,
	}
	if clientIPv6.IsValid() {
		s.IPv6Subnet = serverSettings.IPv6Subnet
		s.IPv6IP = clientIPv6
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
