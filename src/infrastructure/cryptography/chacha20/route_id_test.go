package chacha20

import "testing"

func TestRouteIDFromSessionID(t *testing.T) {
	sessionID := [32]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0xff}

	const want uint64 = 0x0102030405060708
	if got := RouteIDFromSessionID(sessionID); got != want {
		t.Fatalf("RouteIDFromSessionID() = %#x, want %#x", got, want)
	}
}

func TestReadUDPRouteID(t *testing.T) {
	if routeID, ok := ReadUDPRouteID(make([]byte, UDPRouteIDLength-1)); ok || routeID != 0 {
		t.Fatalf("short packet = (%#x, %v), want (0, false)", routeID, ok)
	}

	packet := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0xff}
	const want uint64 = 0x0102030405060708
	if routeID, ok := ReadUDPRouteID(packet); !ok || routeID != want {
		t.Fatalf("valid packet = (%#x, %v), want (%#x, true)", routeID, ok, want)
	}
}
