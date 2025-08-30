package settings

import (
	"encoding/json"
	"errors"
	"strings"
)

var (
	ErrInvalidProtocol = errors.New("invalid protocol")
)

// Protocol specifies the transport protocol
type Protocol int

const (
	UNKNOWN = iota
	TCP
	UDP
	WS
)

func (p Protocol) MarshalJSON() ([]byte, error) {
	var protocolStr string
	switch p {
	case UNKNOWN:
		return json.Marshal("UNKNOWN")
	case TCP:
		protocolStr = "TCP"
	case UDP:
		protocolStr = "UDP"
	case WS:
		protocolStr = "WS"
	default:
		return nil, ErrInvalidProtocol
	}
	return json.Marshal(protocolStr)
}

func (p *Protocol) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch strings.ToUpper(s) {
	case "UNKNOWN":
		*p = UNKNOWN
	case "TCP":
		*p = TCP
	case "UDP":
		*p = UDP
	case "WS":
		*p = WS
	default:
		return ErrInvalidProtocol
	}
	return nil
}

func (p Protocol) String() string {
	switch p {
	case UNKNOWN:
		return "UNKNOWN"
	case TCP:
		return "TCP"
	case UDP:
		return "UDP"
	case WS:
		return "WS"
	default:
		return ErrInvalidProtocol.Error()
	}
}
