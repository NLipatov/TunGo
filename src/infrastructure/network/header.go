package network

import (
	"fmt"
)

type IPHeaderV4 struct {
	data []byte
}

func FromIPPacket(data []byte) IPHeaderV4 {
	return IPHeaderV4{
		data: data,
	}
}

// ReadDestinationAddressBytes reads 32 bits of 'Destination Address' into given buffer
func (h *IPHeaderV4) ReadDestinationAddressBytes(buffer []byte) error {
	if len(buffer) < 4 {
		return fmt.Errorf("invalid buffer size, expected 4 bytes, got %d", len(buffer))
	}

	if len(h.data) < 20 {
		return fmt.Errorf("invalid packet size: %d", len(h.data))
	}

	ipVersion := h.data[0] >> 4
	if ipVersion != 4 {
		return fmt.Errorf("invalid packet version: %d", h.data[0])
	}

	// 16, 17, 18, 19 - destination address bytes
	copy(buffer[:4], h.data[16:20])
	buffer = buffer[:4]

	return nil
}
