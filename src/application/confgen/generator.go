package confgen

import (
	"fmt"
	"net/netip"
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

	serverHost, err := g.resolveServerHost(serverConf.FallbackServerAddress)
	if err != nil {
		return nil, err
	}

	clientID := serverConf.ClientCounter + 1

	clientTCPAddr, clientUDPAddr, clientWSAddr, err := g.allocateClientIPs(serverConf, clientID)
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
		TCPSettings:      deriveClientSettings(serverConf.TCPSettings, clientTCPAddr, serverHost, settings.TCP),
		UDPSettings:      deriveClientSettings(serverConf.UDPSettings, clientUDPAddr, serverHost, settings.UDP),
		WSSettings:       deriveClientSettings(serverConf.WSSettings, clientWSAddr, serverHost, settings.WS),
		X25519PublicKey:  serverConf.X25519PublicKey,
		Protocol:         defaultProtocol,
		ClientPublicKey:  clientPubKey,
		ClientPrivateKey: clientPrivKey[:],
	}

	return &conf, nil
}

func (g *Generator) resolveServerHost(fallback string) (settings.Host, error) {
	defaultIf, err := g.ip.RouteDefault()
	if err != nil {
		return "", err
	}

	defaultIfIpV4, addrErr := g.ip.AddrShowDev(4, defaultIf)
	if addrErr != nil {
		if fallback == "" {
			return "", fmt.Errorf(
				"failed to resolve server IP and no fallback address provided in server configuration: %w",
				addrErr,
			)
		}
		defaultIfIpV4 = fallback
	}

	host, err := settings.NewHost(defaultIfIpV4)
	if err != nil {
		return "", fmt.Errorf("invalid server host %q: %w", defaultIfIpV4, err)
	}
	return host, nil
}

func (g *Generator) allocateClientIPs(
	serverConf *serverConfiguration.Configuration,
	clientID int,
) (tcp, udp, ws netip.Addr, err error) {
	tcp, err = nip.AllocateClientIP(serverConf.TCPSettings.InterfaceSubnet, clientID)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, netip.Addr{}, fmt.Errorf("TCP interface address allocation fail: %w", err)
	}

	udp, err = nip.AllocateClientIP(serverConf.UDPSettings.InterfaceSubnet, clientID)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, netip.Addr{}, fmt.Errorf("UDP interface address allocation fail: %w", err)
	}

	ws, err = nip.AllocateClientIP(serverConf.WSSettings.InterfaceSubnet, clientID)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, netip.Addr{}, fmt.Errorf("WS interface address allocation fail: %w", err)
	}

	if err = g.serverConfigurationManager.IncrementClientCounter(); err != nil {
		return netip.Addr{}, netip.Addr{}, netip.Addr{}, err
	}

	return tcp, udp, ws, nil
}

func deriveClientSettings(
	serverSettings settings.Settings,
	clientIP netip.Addr,
	serverHost settings.Host,
	protocol settings.Protocol,
) settings.Settings {
	mtu := serverSettings.MTU
	if protocol == settings.UDP {
		mtu = settings.SafeMTU
	}
	return settings.Settings{
		InterfaceName:   serverSettings.InterfaceName,
		InterfaceSubnet: serverSettings.InterfaceSubnet,
		InterfaceIP:     clientIP.Unmap(),
		Host:            serverHost,
		Port:            serverSettings.Port,
		MTU:             mtu,
		Protocol:        protocol,
		Encryption:      serverSettings.Encryption,
		DialTimeoutMs:   serverSettings.DialTimeoutMs,
	}
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
