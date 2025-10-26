package handshake

import (
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"net"
	"tungo/domain/network/ip/packet_validation"
	"tungo/infrastructure/network/ip"
)

type ClientHello struct {
	ipVersion      ip.Version
	ipAddress      net.IP
	ipValidator    packet_validation.IPValidator
	edPublicKey    ed25519.PublicKey
	curvePublicKey []byte
	nonce          []byte
	mtu            uint16
	hasMTU         bool
}

func NewClientHello(
	IpVersion ip.Version,
	IpAddress net.IP,
	EdPublicKey ed25519.PublicKey,
	CurvePublicKey []byte,
	ClientNonce []byte,
	ipValidator packet_validation.IPValidator,
	mtu uint16,
) ClientHello {
	return ClientHello{
		ipVersion:      IpVersion,
		ipAddress:      IpAddress,
		edPublicKey:    EdPublicKey,
		curvePublicKey: CurvePublicKey,
		nonce:          ClientNonce,
		ipValidator:    ipValidator,
		mtu:            mtu,
		hasMTU:         true,
	}
}

func NewEmptyClientHelloWithDefaultIPValidator() ClientHello {
	return ClientHello{
		ipValidator: packet_validation.NewDefaultIPValidator(
			packet_validation.Policy{
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

func (c *ClientHello) MTU() (uint16, bool) {
	if !c.hasMTU {
		return 0, false
	}
	return c.mtu, true
}

func (c *ClientHello) MarshalBinary() ([]byte, error) {
	if err := c.validateIP(); err != nil {
		return nil, err
	}

	ipLength := len(c.ipAddress)
	totalLength := lengthHeaderLength + ipLength + curvePublicKeyLength + curvePublicKeyLength + nonceLength
	if c.hasMTU {
		totalLength += mtuFieldLength
	}

	arr := make([]byte, totalLength)
	arr[0] = c.ipVersion.Byte()
	arr[1] = uint8(ipLength)

	offset := lengthHeaderLength
	copy(arr[offset:], c.ipAddress)
	offset += ipLength
	copy(arr[offset:], c.edPublicKey)
	offset += curvePublicKeyLength
	copy(arr[offset:], c.curvePublicKey)
	offset += curvePublicKeyLength
	copy(arr[offset:], c.nonce)
	offset += nonceLength

	if c.hasMTU {
		binary.BigEndian.PutUint16(arr[offset:], c.mtu)
	}

	return arr, nil
}

func (c *ClientHello) UnmarshalBinary(data []byte) error {
	if len(data) < minClientHelloSizeBytes || len(data) > MaxClientHelloSizeBytes {
		return fmt.Errorf("invalid message length")
	}

	c.ipVersion = ip.Version(data[0])
	if !c.ipVersion.Valid() {
		return fmt.Errorf("invalid IP version: %d", c.ipVersion)
	}

	ipAddressLength := int(data[1])
	if ipAddressLength+lengthHeaderLength > len(data) {
		return fmt.Errorf("invalid Client Hello size: %d", len(data))
	}

	c.ipAddress = data[lengthHeaderLength : lengthHeaderLength+ipAddressLength]

	c.edPublicKey = data[lengthHeaderLength+ipAddressLength : lengthHeaderLength+ipAddressLength+curvePublicKeyLength]

	c.curvePublicKey = data[lengthHeaderLength+ipAddressLength+curvePublicKeyLength : lengthHeaderLength+ipAddressLength+curvePublicKeyLength+curvePublicKeyLength]

	end := lengthHeaderLength + ipAddressLength + curvePublicKeyLength + curvePublicKeyLength + nonceLength
	c.nonce = data[lengthHeaderLength+ipAddressLength+curvePublicKeyLength+curvePublicKeyLength : end]

	if len(data) == end {
		c.hasMTU = false
		c.mtu = 0
		return c.validateIP()
	}

	if len(data) != end+mtuFieldLength {
		return fmt.Errorf("invalid Client Hello size: %d", len(data))
	}

	c.hasMTU = true
	c.mtu = binary.BigEndian.Uint16(data[end:])

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
