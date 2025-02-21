package settings

import (
	"encoding/json"
	"errors"
	"strings"
)

type Protocol int

const (
	TCP = iota
	UDP
)

type ConnectionSettings struct {
	InterfaceName    string `json:"InterfaceName"`
	InterfaceIPCIDR  string `json:"InterfaceIPCIDR"`
	InterfaceAddress string `json:"InterfaceAddress"`
	ConnectionIP     string `json:"ConnectionIP"`
	Port             string `json:"Port"`
	MTU              int    `json:"MTU"`
	Protocol         Protocol
	DialTimeoutMs    int `json:"DialTimeoutMs"`
}

func (p *Protocol) MarshalJSON() ([]byte, error) {
	var protocolStr string
	switch *p {
	case TCP:
		protocolStr = "tcp"
	case UDP:
		protocolStr = "udp"
	default:
		return nil, errors.New("invalid protocol")
	}
	return json.Marshal(protocolStr)
}

func (p *Protocol) UnmarshalJSON(data []byte) error {
	var protocolStr string
	if err := json.Unmarshal(data, &protocolStr); err != nil {
		return err
	}
	switch strings.ToLower(protocolStr) {
	case "tcp":
		*p = TCP
	case "udp":
		*p = UDP
	default:
		return errors.New("invalid protocol")
	}
	return nil
}
