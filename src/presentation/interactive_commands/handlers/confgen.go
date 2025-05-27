package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/linux/network_tools/ip"
	"tungo/infrastructure/network"
	"tungo/settings"
	"tungo/settings/client_configuration"
	"tungo/settings/server_configuration"
)

type ConfgenHandler struct {
	ipWrapper ip.Contract
}

func NewConfgenHandler() *ConfgenHandler {
	return &ConfgenHandler{
		ipWrapper: ip.NewWrapper(PAL.NewExecCommander()),
	}
}

func (c *ConfgenHandler) GenerateNewClientConf() error {
	newConf, err := c.generate()
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
func (c *ConfgenHandler) generate() (*client_configuration.Configuration, error) {
	serverConfigurationManager := server_configuration.NewManager(server_configuration.NewServerResolver())
	serverConf, err := serverConfigurationManager.Configuration()
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration: %s", err)
	}

	defaultIf, err := c.ipWrapper.RouteDefault()
	if err != nil {
		return nil, err
	}

	defaultIfIpV4, addressResolutionError := c.ipWrapper.AddrShowDev(4, defaultIf)
	if addressResolutionError != nil {
		if serverConf.FallbackServerAddress == "" {
			return nil, fmt.Errorf("failed to resolve server IP and no fallback address provided in server configuration: %s", addressResolutionError)
		}
		defaultIfIpV4 = serverConf.FallbackServerAddress
	}

	IncrementedClientCounter := serverConf.ClientCounter + 1
	clientTCPIfIp, err := network.AllocateClientIp(serverConf.TCPSettings.InterfaceIPCIDR, IncrementedClientCounter)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate client's TCP IP address: %s", err)
	}

	clientUIDPIfIp, err := network.AllocateClientIp(serverConf.UDPSettings.InterfaceIPCIDR, IncrementedClientCounter)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate client's TCP IP address: %s", err)
	}

	err = serverConfigurationManager.IncrementClientCounter()
	if err != nil {
		return nil, err
	}

	conf := client_configuration.Configuration{
		TCPSettings: settings.ConnectionSettings{
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
		UDPSettings: settings.ConnectionSettings{
			InterfaceName:    serverConf.UDPSettings.InterfaceName,
			InterfaceIPCIDR:  serverConf.UDPSettings.InterfaceIPCIDR,
			InterfaceAddress: clientUIDPIfIp,
			ConnectionIP:     defaultIfIpV4,
			Port:             serverConf.UDPSettings.Port,
			MTU:              serverConf.UDPSettings.MTU,
			Protocol:         settings.UDP,
			Encryption:       serverConf.UDPSettings.Encryption,
			DialTimeoutMs:    serverConf.UDPSettings.DialTimeoutMs,
		},
		Ed25519PublicKey: serverConf.Ed25519PublicKey,
		Protocol:         c.getDefaultProtocol(serverConf),
	}

	return &conf, nil
}

func (c *ConfgenHandler) getDefaultProtocol(conf *server_configuration.Configuration) settings.Protocol {
	if conf.EnableUDP {
		return settings.UDP
	}

	return settings.TCP
}
