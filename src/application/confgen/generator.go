package confgen

import (
	"fmt"
	"net/netip"
	"strings"
	"tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/PAL/exec_commander"
	"tungo/infrastructure/PAL/linux/network_tools/ip"
	"tungo/infrastructure/cryptography/primitives"
	nip "tungo/infrastructure/network/ip"
	"tungo/infrastructure/settings"
)

type Generator struct {
	ip                         ip.Contract
	serverConfigurationManager serverConfiguration.ConfigurationManager
	keyDeriver                 primitives.KeyDeriver
}

func NewGenerator(
	serverConfigurationManager serverConfiguration.ConfigurationManager,
	keyDeriver primitives.KeyDeriver,
) *Generator {
	return &Generator{
		ip: ip.NewWrapper(
			exec_commander.NewExecCommander(),
		),
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

	serverHost, ipv6Host, err := g.resolveServerHosts(serverConf.FallbackServerAddress)
	if err != nil {
		return nil, err
	}

	if !ipv6Host.IsZero() {
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
		TCPSettings:      deriveClientSettings(serverConf.TCPSettings, clientIPs.tcp, clientIPs.tcpV6, serverHost, ipv6Host, settings.TCP),
		UDPSettings:      deriveClientSettings(serverConf.UDPSettings, clientIPs.udp, clientIPs.udpV6, serverHost, ipv6Host, settings.UDP),
		WSSettings:       deriveClientSettings(serverConf.WSSettings, clientIPs.ws, clientIPs.wsV6, serverHost, ipv6Host, settings.WS),
		X25519PublicKey:  serverConf.X25519PublicKey,
		Protocol:         defaultProtocol,
		ClientPublicKey:  clientPubKey,
		ClientPrivateKey: clientPrivKey[:],
	}

	return &conf, nil
}

func (g *Generator) resolveServerHosts(fallback string) (ipv4Host, ipv6Host settings.Host, err error) {
	defaultIf, routeErr := g.ip.RouteDefault()
	if routeErr != nil {
		return "", "", routeErr
	}

	defaultIfIpV4, addrErr := g.ip.AddrShowDev(4, defaultIf)
	if addrErr != nil {
		if fallback == "" {
			return "", "", fmt.Errorf(
				"failed to resolve server IP and no fallback address provided in server configuration: %w",
				addrErr,
			)
		}
		defaultIfIpV4 = fallback
	}

	ipv4Host, err = settings.NewHost(defaultIfIpV4)
	if err != nil {
		return "", "", fmt.Errorf("invalid server host %q: %w", defaultIfIpV4, err)
	}

	ipv6Str, ipv6Err := g.ip.AddrShowDev(6, defaultIf)
	if ipv6Err == nil {
		for _, line := range strings.Split(ipv6Str, "\n") {
			addr := strings.TrimSpace(line)
			if addr == "" {
				continue
			}
			ip, parseErr := netip.ParseAddr(addr)
			if parseErr != nil {
				continue
			}
			if ip.IsLinkLocalUnicast() {
				continue
			}
			ipv6Host, _ = settings.NewHost(addr)
			break
		}
	}

	return ipv4Host, ipv6Host, nil
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

	ips.tcp, err = nip.AllocateClientIP(serverConf.TCPSettings.InterfaceSubnet, clientID)
	if err != nil {
		return clientIPs{}, fmt.Errorf("TCP interface address allocation fail: %w", err)
	}

	ips.udp, err = nip.AllocateClientIP(serverConf.UDPSettings.InterfaceSubnet, clientID)
	if err != nil {
		return clientIPs{}, fmt.Errorf("UDP interface address allocation fail: %w", err)
	}

	ips.ws, err = nip.AllocateClientIP(serverConf.WSSettings.InterfaceSubnet, clientID)
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
	ipv6Host settings.Host,
	protocol settings.Protocol,
) settings.Settings {
	mtu := serverSettings.MTU
	if protocol == settings.UDP {
		mtu = settings.SafeMTU
	}
	s := settings.Settings{
		InterfaceName:   serverSettings.InterfaceName,
		InterfaceSubnet: serverSettings.InterfaceSubnet,
		InterfaceIP:     clientIP.Unmap(),
		Host:            serverHost,
		IPv6Host:        ipv6Host,
		Port:            serverSettings.Port,
		MTU:             mtu,
		Protocol:        protocol,
		Encryption:      serverSettings.Encryption,
		DialTimeoutMs:   serverSettings.DialTimeoutMs,
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
