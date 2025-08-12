package handshake

import (
	"crypto/ed25519"
	"fmt"
	"net"
	"tungo/application"
	"tungo/domain/network/ip"
	"tungo/infrastructure/network"
)

type ClientHello struct {
	ipVersion      uint8
	ipAddress      net.IP
	ipValidator    application.IPValidator
	edPublicKey    ed25519.PublicKey
	curvePublicKey []byte
	nonce          []byte
}

func NewClientHello(
	IpVersion uint8,
	IpAddress net.IP,
	EdPublicKey ed25519.PublicKey,
	CurvePublicKey []byte,
	ClientNonce []byte,
	ipValidator application.IPValidator,
) ClientHello {
	return ClientHello{
		ipVersion:      IpVersion,
		ipAddress:      IpAddress,
		edPublicKey:    EdPublicKey,
		curvePublicKey: CurvePublicKey,
		nonce:          ClientNonce,
		ipValidator:    ipValidator,
	}
}

func NewEmptyClientHelloWithDefaultIPValidator() ClientHello {
	return ClientHello{
		ipValidator: network.NewIPValidator(
			ip.ValidationPolicy{
				AllowV4:           true,
				AllowV6:           true,
				RequirePrivate:    true,
				ForbidLoopback:    true,
				ForbidMulticast:   true,
				ForbidUnspecified: true,
				ForbidLinkLocal:   true,
				ForbidBroadcastV4: true,
			},
		),
	}
}

func (c *ClientHello) Nonce() []byte {
	return c.nonce
}

func (c *ClientHello) CurvePublicKey() []byte {
	return c.curvePublicKey
}

func (c *ClientHello) MarshalBinary() ([]byte, error) {
	if err := c.validateIP(); err != nil {
		return nil, err
	}

	arr := make([]byte, lengthHeaderLength+len(c.ipAddress)+curvePublicKeyLength+curvePublicKeyLength+nonceLength)
	arr[0] = c.ipVersion
	arr[1] = uint8(len(c.ipAddress))
	copy(arr[lengthHeaderLength:], c.ipAddress)
	copy(arr[lengthHeaderLength+len(c.ipAddress):], c.edPublicKey)
	copy(arr[lengthHeaderLength+len(c.ipAddress)+curvePublicKeyLength:], c.curvePublicKey)
	copy(arr[lengthHeaderLength+len(c.ipAddress)+curvePublicKeyLength+curvePublicKeyLength:], c.nonce)

	return arr, nil
}

func (c *ClientHello) UnmarshalBinary(data []byte) error {
	if len(data) < minClientHelloSizeBytes || len(data) > MaxClientHelloSizeBytes {
		return fmt.Errorf("invalid message length")
	}

	ipV := ip.Version(data[0])
	if ipV != ip.V4 && ipV != ip.V6 {
		return fmt.Errorf("invalid IP version: %d", c.ipVersion)
	}
	c.ipVersion = data[0]

	ipAddressLength := data[1]
	if int(ipAddressLength+lengthHeaderLength) > len(data) {
		return fmt.Errorf("invalid Client Hello size: %d", len(data))
	}

	c.ipAddress = data[lengthHeaderLength : lengthHeaderLength+ipAddressLength]

	c.edPublicKey = data[lengthHeaderLength+ipAddressLength : lengthHeaderLength+ipAddressLength+curvePublicKeyLength]

	c.curvePublicKey = data[lengthHeaderLength+ipAddressLength+curvePublicKeyLength : lengthHeaderLength+ipAddressLength+curvePublicKeyLength+curvePublicKeyLength]

	c.nonce = data[lengthHeaderLength+ipAddressLength+curvePublicKeyLength+curvePublicKeyLength : lengthHeaderLength+ipAddressLength+curvePublicKeyLength+curvePublicKeyLength+nonceLength]

	return c.validateIP()
}

func (c *ClientHello) validateIP() error {
	switch c.ipVersion {
	case 4:
		if vErr := c.ipValidator.ValidateIP(ip.V4, c.ipAddress); vErr != nil {
			return vErr
		}
	case 6:
		if vErr := c.ipValidator.ValidateIP(ip.V6, c.ipAddress); vErr != nil {
			return vErr
		}
	default:
		return fmt.Errorf("invalid ip version: %d", c.ipVersion)
	}
	return nil
}
