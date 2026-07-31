package chacha20

import (
	"bytes"
	"testing"
)

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

func TestPrependUDPRouteID(t *testing.T) {
	const routeID uint64 = 0x0102030405060708
	want := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0xaa, 0xbb}

	t.Run("allocates when capacity is insufficient", func(t *testing.T) {
		packet := []byte{0xaa, 0xbb}
		got := prependUDPRouteID(packet, routeID)

		if !bytes.Equal(got, want) {
			t.Fatalf("prependUDPRouteID() = %x, want %x", got, want)
		}
		if !bytes.Equal(packet, []byte{0xaa, 0xbb}) {
			t.Fatalf("input packet was modified: %x", packet)
		}
	})

	t.Run("reuses packet capacity", func(t *testing.T) {
		backing := make([]byte, 2, 2+UDPRouteIDLength)
		copy(backing, []byte{0xaa, 0xbb})

		got := prependUDPRouteID(backing, routeID)
		if !bytes.Equal(got, want) {
			t.Fatalf("prependUDPRouteID() = %x, want %x", got, want)
		}
		if &got[0] != &backing[0] {
			t.Fatal("prependUDPRouteID() allocated despite sufficient capacity")
		}
	})
}
