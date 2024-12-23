package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"tungo/Domain/settings"
	"tungo/Domain/settings/client"
	"tungo/Domain/settings/server"
	ip2 "tungo/Infrastructure/network/ip"
)

func GenerateNewClientConf() error {
	newConf, err := generate()
	if err != nil {
		log.Fatalf("failed to generate client conf: %s\n", err)
	}

	marshalled, err := json.MarshalIndent(newConf, "", "  ")
	if err != nil {
		log.Fatalf("failed to marshalize client conf: %s\n", err)
	}

	fmt.Println(string(marshalled))
	return nil
}

// generate generates new client configuration
func generate() (*client.Conf, error) {
	serverConf, err := (&server.Conf{}).Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration: %s", err)
	}

	defaultIf, err := ip2.RouteDefault()
	if err != nil {
		return nil, err
	}

	defaultIfIpV4, addressResolutionError := ip2.InterfaceIpAddr(4, defaultIf)
	if addressResolutionError != nil {
		if serverConf.FallbackServerAddress == "" {
			return nil, fmt.Errorf("failed to resolve server IP and no fallback address provided in server configuration: %s", addressResolutionError)
		}
		defaultIfIpV4 = serverConf.FallbackServerAddress
	}

	IncrementedClientCounter := serverConf.ClientCounter + 1
	clientTCPIfIp, err := ip2.AllocateClientIp(serverConf.TCPSettings.InterfaceIPCIDR, IncrementedClientCounter)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate client's TCP IP address: %s", err)
	}

	clientUIDPIfIp, err := ip2.AllocateClientIp(serverConf.UDPSettings.InterfaceIPCIDR, IncrementedClientCounter)
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
			Protocol:         settings.TCP,
		},
		UDPSettings: settings.ConnectionSettings{
			InterfaceName:    serverConf.UDPSettings.InterfaceName,
			InterfaceIPCIDR:  serverConf.UDPSettings.InterfaceIPCIDR,
			InterfaceAddress: clientUIDPIfIp,
			ConnectionIP:     defaultIfIpV4,
			Port:             serverConf.UDPSettings.Port,
			MTU:              serverConf.UDPSettings.MTU,
			Protocol:         settings.UDP,
		},
		Ed25519PublicKey:          serverConf.Ed25519PublicKey,
		TCPWriteChannelBufferSize: 1000,
		UDPNonceRingBufferSize:    serverConf.UDPNonceRingBufferSize,
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
