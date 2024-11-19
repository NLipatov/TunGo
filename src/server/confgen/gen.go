package confgen

import (
	"fmt"
	"tungo/network/ip"
	"tungo/settings"
	"tungo/settings/client"
	"tungo/settings/server"
)

// Generate generates new client configuration
func Generate() (*client.Conf, error) {
	serverConf, err := (&server.Conf{}).Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration: %s", err)
	}

	defaultIf, err := ip.RouteDefault()
	if err != nil {
		return nil, err
	}

	defaultIfIpV4, addressResolutionError := ip.InterfaceIpAddr(4, defaultIf)
	if addressResolutionError != nil {
		if serverConf.FallbackServerAddress == "" {
			return nil, fmt.Errorf("failed to resolve server IP and no fallback address provided in server configuration: %s", addressResolutionError)
		}
		defaultIfIpV4 = serverConf.FallbackServerAddress
	}

	IncrementedClientCounter := serverConf.ClientCounter + 1
	clientTCPIfIp, err := ip.AllocateClientIp(serverConf.TCPSettings.InterfaceIPCIDR, IncrementedClientCounter)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate client's TCP IP address: %s", err)
	}

	clientUIDPIfIp, err := ip.AllocateClientIp(serverConf.UDPSettings.InterfaceIPCIDR, IncrementedClientCounter)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate client's TCP IP address: %s", err)
	}

	serverConf.ClientCounter = IncrementedClientCounter
	err = serverConf.RewriteConf()
	if err != nil {
		return nil, err
	}

	conf := client.Conf{
		TCPSettings: settings.ConnectionSettings{
			InterfaceName:    serverConf.TCPSettings.InterfaceName,
			InterfaceIPCIDR:  serverConf.TCPSettings.InterfaceIPCIDR,
			InterfaceAddress: clientTCPIfIp,
			ConnectionIP:     defaultIfIpV4,
			Port:             serverConf.TCPSettings.Port,
			MTU:              serverConf.TCPSettings.MTU,
			SessionMarker:    serverConf.TCPSettings.SessionMarker,
			Protocol:         settings.TCP,
		},
		UDPSettings: settings.ConnectionSettings{
			InterfaceName:    serverConf.UDPSettings.InterfaceName,
			InterfaceIPCIDR:  serverConf.UDPSettings.InterfaceIPCIDR,
			InterfaceAddress: clientUIDPIfIp,
			ConnectionIP:     defaultIfIpV4,
			Port:             serverConf.UDPSettings.Port,
			MTU:              serverConf.UDPSettings.MTU,
			SessionMarker:    serverConf.UDPSettings.SessionMarker,
			Protocol:         settings.UDP,
		},
		Ed25519PublicKey:          serverConf.Ed25519PublicKey,
		TCPWriteChannelBufferSize: 1000,
		Protocol:                  getDefaultProtocol(serverConf),
	}

	return &conf, nil
}

func getDefaultProtocol(conf *server.Conf) settings.Protocol {
	if conf.EnableUDP {
		return settings.UDP
	}

	return settings.TCP
}
