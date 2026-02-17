package chacha20

import (
	"encoding/binary"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	UDPRouteIDLength = 8
	UDPNonceOffset   = UDPRouteIDLength
	UDPEpochOffset   = UDPNonceOffset + NonceEpochOffset
	UDPMinPacketSize = UDPRouteIDLength + chacha20poly1305.NonceSize + chacha20poly1305.Overhead
)

func RouteIDFromSessionID(sessionID [32]byte) uint64 {
	return binary.BigEndian.Uint64(sessionID[:UDPRouteIDLength])
}

func ReadUDPRouteID(packet []byte) (uint64, bool) {
	if len(packet) < UDPRouteIDLength {
		return 0, false
	}
	return binary.BigEndian.Uint64(packet[:UDPRouteIDLength]), true
}

func prependUDPRouteID(packet []byte, routeID uint64) []byte {
	originalLen := len(packet)
	if cap(packet) < originalLen+UDPRouteIDLength {
		out := make([]byte, originalLen+UDPRouteIDLength)
		binary.BigEndian.PutUint64(out[:UDPRouteIDLength], routeID)
		copy(out[UDPRouteIDLength:], packet)
		return out
	}

	packet = packet[:originalLen+UDPRouteIDLength]
	copy(packet[UDPRouteIDLength:], packet[:originalLen])
	binary.BigEndian.PutUint64(packet[:UDPRouteIDLength], routeID)
	return packet
}
