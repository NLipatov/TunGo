package settings

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"
)

type Settings struct {
	InterfaceName string       `json:"InterfaceName"`
	IPv4Subnet    netip.Prefix `json:"IPv4Subnet"`
	IPv4IP        netip.Addr   `json:"IPv4IP"`
	IPv6Subnet    netip.Prefix `json:"IPv6Subnet,omitzero"`
	IPv6IP        netip.Addr   `json:"IPv6IP,omitzero"`
	Host          Host         `json:"Host"`
	Port          int          `json:"Port"`
	MTU           int          `json:"MTU"`
	Protocol      Protocol     `json:"Protocol"`
	Encryption    Encryption   `json:"Encryption"`
	DialTimeoutMs DialTimeoutMs `json:"DialTimeoutMs"`
}

// UnmarshalJSON supports both int and legacy string representation for Port.
func (s *Settings) UnmarshalJSON(data []byte) error {
	type Alias Settings
	aux := &struct {
		Port json.RawMessage `json:"Port"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if len(aux.Port) == 0 {
		return nil
	}
	// Try int first
	var portInt int
	if err := json.Unmarshal(aux.Port, &portInt); err == nil {
		s.Port = portInt
		return nil
	}
	// Fall back to string (legacy configs)
	var portStr string
	if err := json.Unmarshal(aux.Port, &portStr); err != nil {
		return fmt.Errorf("Port must be an integer or a string, got %s", string(aux.Port))
	}
	parsed, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid Port string %q: %w", portStr, err)
	}
	s.Port = parsed
	return nil
}
