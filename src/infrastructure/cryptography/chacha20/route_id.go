package chacha20

import "encoding/binary"

func RouteIDFromSessionID(sessionID [32]byte) uint64 {
	return binary.BigEndian.Uint64(sessionID[:UDPRouteIDLength])
}

func ReadUDPRouteID(packet []byte) (uint64, bool) {
	if len(packet) < UDPRouteIDLength {
		return 0, false
	}
	return binary.BigEndian.Uint64(packet[:UDPRouteIDLength]), true
}
