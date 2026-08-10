package udp

import "encoding/binary"

func RouteIDFromSessionID(sessionID [32]byte) uint64 {
	return binary.BigEndian.Uint64(sessionID[:RouteIDLength])
}

func ReadRouteID(packet []byte) (uint64, bool) {
	if len(packet) < RouteIDLength {
		return 0, false
	}
	return binary.BigEndian.Uint64(packet[:RouteIDLength]), true
}
