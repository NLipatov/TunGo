package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/PAL/linux/network_tools/ip"
	nip "tungo/infrastructure/network/ip"
	"tungo/infrastructure/settings"
)

type ConfgenHandler struct {
	ipWrapper  ip.Contract
	cfgManager serverConfiguration.ServerConfigurationManager
}

func NewConfgenHandler(manager serverConfiguration.ServerConfigurationManager) *ConfgenHandler {
	return &ConfgenHandler{
		ipWrapper:  ip.NewWrapper(PAL.NewExecCommander()),
		cfgManager: manager,
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
func (c *ConfgenHandler) generate() (*client.Configuration, error) {
	serverConf, err := c.cfgManager.Configuration()
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
	clientTCPIfIp, err := nip.AllocateClientIp(serverConf.TCPSettings.InterfaceIPCIDR, IncrementedClientCounter)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate client's TCP IP address: %s", err)
	}

	clientUIDPIfIp, err := nip.AllocateClientIp(serverConf.UDPSettings.InterfaceIPCIDR, IncrementedClientCounter)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate client's TCP IP address: %s", err)
	}

	err = c.cfgManager.IncrementClientCounter()
	if err != nil {
		return nil, err
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

func (c *ConfgenHandler) getDefaultProtocol(conf *serverConfiguration.Configuration) settings.Protocol {
	if conf.EnableUDP {
		return settings.UDP
	}

	return settings.TCP
}
