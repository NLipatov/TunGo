package handlers

import (
	"fmt"
	"tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/PAL/exec_commander"
	"tungo/infrastructure/PAL/linux/network_tools/ip"
	"tungo/infrastructure/cryptography/primitives"
	nip "tungo/infrastructure/network/ip"
	"tungo/infrastructure/settings"
)

type ConfgenHandler struct {
	ip                         ip.Contract
	serverConfigurationManager serverConfiguration.ConfigurationManager
	marshaller                 JsonMarshaller
}

func NewConfgenHandler(
	serverConfigurationManager serverConfiguration.ConfigurationManager,
	marshaller JsonMarshaller,
) *ConfgenHandler {
	return &ConfgenHandler{
		ip: ip.NewWrapper(
			exec_commander.NewExecCommander(),
		),
		serverConfigurationManager: serverConfigurationManager,
		marshaller:                 marshaller,
	}
}

func (c *ConfgenHandler) GenerateNewClientConf() error {
	newConf, err := c.generate()
	if err != nil {
		return fmt.Errorf("failed to generate client conf: %w", err)
	}

	marshalled, err := c.marshaller.MarshalIndent(newConf, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshalize client conf: %w", err)
	}

	fmt.Println(string(marshalled))
	return nil
}

// generate generates new client configuration
func (c *ConfgenHandler) generate() (*client.Configuration, error) {
	serverConf, err := c.serverConfigurationManager.Configuration()
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration: %w", err)
	}

	defaultIf, err := c.ip.RouteDefault()
	if err != nil {
		return nil, err
	}

	defaultIfIpV4, addressResolutionError := c.ip.AddrShowDev(4, defaultIf)
	if addressResolutionError != nil {
		if serverConf.FallbackServerAddress == "" {
			return nil, fmt.Errorf(
				"failed to resolve server IP and no fallback address provided in server configuration: %w",
				addressResolutionError,
			)
		}
		defaultIfIpV4 = serverConf.FallbackServerAddress
	}

	clientTCPIfIp, clientUDPIfIp, clientWSIfIp, err := c.allocateNewClientIP(serverConf)
	if err != nil {
		return nil, err
	}

	// Generate client X25519 keypair for Noise IK handshake.
	keyDeriver := &primitives.DefaultKeyDeriver{}
	clientPubKey, clientPrivKey, err := keyDeriver.GenerateX25519KeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate client keypair: %w", err)
	}

	// Determine internal IP based on default protocol.
	defaultProtocol := c.getDefaultProtocol(serverConf)
	var internalIP string
	switch defaultProtocol {
	case settings.UDP:
		internalIP = clientUDPIfIp
	case settings.TCP:
		internalIP = clientTCPIfIp
	case settings.WS, settings.WSS:
		internalIP = clientWSIfIp
	}

	// Add the new client to server's AllowedPeers.
	newPeer := serverConfiguration.AllowedPeer{
		PublicKey:  clientPubKey,
		Enabled:    true,
		ClientIP:   internalIP,
		AllowedIPs: nil,
	}
	if err := c.serverConfigurationManager.AddAllowedPeer(newPeer); err != nil {
		return nil, fmt.Errorf("failed to add client to AllowedPeers: %w", err)
	}

	conf := client.Configuration{
		TCPSettings: settings.Settings{
			InterfaceName:    serverConf.TCPSettings.InterfaceName,
			InterfaceIPCIDR:  serverConf.TCPSettings.InterfaceIPCIDR,
			InterfaceAddress: clientTCPIfIp,
			ConnectionIP:     defaultIfIpV4,
			Port:             serverConf.TCPSettings.Port,
			MTU:              serverConf.TCPSettings.MTU,
			Protocol:         settings.TCP,
			Encryption:       serverConf.TCPSettings.Encryption,
			DialTimeoutMs:    serverConf.TCPSettings.DialTimeoutMs,
		},
		UDPSettings: settings.Settings{
			InterfaceName:    serverConf.UDPSettings.InterfaceName,
			InterfaceIPCIDR:  serverConf.UDPSettings.InterfaceIPCIDR,
			InterfaceAddress: clientUDPIfIp,
			ConnectionIP:     defaultIfIpV4,
			Port:             serverConf.UDPSettings.Port,
			MTU:              settings.SafeMTU,
			Protocol:         settings.UDP,
			Encryption:       serverConf.UDPSettings.Encryption,
			DialTimeoutMs:    serverConf.UDPSettings.DialTimeoutMs,
		},
		WSSettings: settings.Settings{
			InterfaceName:    serverConf.WSSettings.InterfaceName,
			InterfaceIPCIDR:  serverConf.WSSettings.InterfaceIPCIDR,
			InterfaceAddress: clientWSIfIp,
			ConnectionIP:     defaultIfIpV4,
			Port:             serverConf.WSSettings.Port,
			MTU:              serverConf.WSSettings.MTU,
			Protocol:         settings.WS,
			Encryption:       serverConf.WSSettings.Encryption,
			DialTimeoutMs:    serverConf.WSSettings.DialTimeoutMs,
		},
		X25519PublicKey:  serverConf.X25519PublicKey,
		Protocol:         defaultProtocol,
		ClientPublicKey:  clientPubKey,
		ClientPrivateKey: clientPrivKey[:],
		InternalIP:       internalIP,
	}

	return &conf, nil
}

func (c *ConfgenHandler) allocateNewClientIP(
	serverConfiguration *serverConfiguration.Configuration,
) (
	tcpIfIp string, // client TCP interface address
	udpIfIp string, // client UDP interface address
	wsIfIp string, // client WS interface address
	err error,
) {
	clientCounter := serverConfiguration.ClientCounter + 1
	var e error
	tcpIfIp, e = nip.AllocateClientIp(serverConfiguration.TCPSettings.InterfaceIPCIDR, clientCounter)
	if e != nil {
		return "", "", "", fmt.Errorf("TCP interface address allocation fail: %w", e)
	}

	udpIfIp, e = nip.AllocateClientIp(serverConfiguration.UDPSettings.InterfaceIPCIDR, clientCounter)
	if e != nil {
		return "", "", "", fmt.Errorf("UDP interface address allocation fail: %w", e)
	}

	wsIfIp, e = nip.AllocateClientIp(serverConfiguration.WSSettings.InterfaceIPCIDR, clientCounter)
	if e != nil {
		return "", "", "", fmt.Errorf("WS interface address allocation fail: %w", e)
	}

	e = c.serverConfigurationManager.IncrementClientCounter()
	if e != nil {
		return "", "", "", e
	}

	return tcpIfIp, udpIfIp, wsIfIp, e
}

func (c *ConfgenHandler) getDefaultProtocol(conf *serverConfiguration.Configuration) settings.Protocol {
	if conf.EnableUDP {
		return settings.UDP
	}

	if conf.EnableTCP {
		return settings.TCP
	}

	return settings.WS
}
