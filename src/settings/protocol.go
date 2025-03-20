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

func (p Protocol) MarshalJSON() ([]byte, error) {
	var protocolStr string
	switch p {
	case TCP:
		protocolStr = "TCP"
	case UDP:
		protocolStr = "UDP"
	default:
		return nil, errors.New("invalid protocol")
	}
	return json.Marshal(protocolStr)
}

func (p *Protocol) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch strings.ToUpper(s) {
	case "TCP":
		*p = TCP
	case "UDP":
		*p = UDP
	default:
		return errors.New("invalid protocol")
	}
	return nil
}
