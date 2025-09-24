package ip

import (
	"testing"

	"net/netip"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

func mkIPv4(ihlWords int, dst [4]byte) []byte {
	if ihlWords < 5 {
		ihlWords = 5
	}
	hlen := ihlWords * 4
	h := make([]byte, hlen)
	h[0] = byte(0x40 | (ihlWords & 0x0F)) // v=4, IHL=ihlWords
	copy(h[16:20], dst[:])
	return h
}

func mkIPv6(dst [16]byte) []byte {
	h := make([]byte, ipv6.HeaderLen)
	h[0] = 0x60 // v=6
	copy(h[24:40], dst[:])
	return h
}

func TestDestinationAddress_IPv4_OK(t *testing.T) {
	p := NewHeaderParser()
	want := netip.AddrFrom4([4]byte{1, 2, 3, 4})
	got, err := p.DestinationAddress(mkIPv4(5, [4]byte{1, 2, 3, 4}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDestinationAddress_IPv4_WithOptions_OK(t *testing.T) {
	p := NewHeaderParser()
	want := netip.AddrFrom4([4]byte{9, 8, 7, 6})
	h := mkIPv4(6, [4]byte{9, 8, 7, 6})
	for i := 20; i < len(h); i++ {
		h[i] = byte(i)
	}
	got, err := p.DestinationAddress(h)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDestinationAddress_IPv6_OK(t *testing.T) {
	p := NewHeaderParser()
	var dst [16]byte
	for i := range dst {
		dst[i] = byte(i + 1)
	}
	want := netip.AddrFrom16(dst)
	got, err := p.DestinationAddress(mkIPv6(dst))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDestinationAddress_Errors(t *testing.T) {
	p := NewHeaderParser()

	t.Run("empty", func(t *testing.T) {
		if _, err := p.DestinationAddress(nil); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("invalid version nibble", func(t *testing.T) {
		h := make([]byte, ipv4.HeaderLen)
		h[0] = 0x50 // v=5
		if _, err := p.DestinationAddress(h); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("ipv4 too small", func(t *testing.T) {
		h := make([]byte, 10)
		h[0] = 0x45
		if _, err := p.DestinationAddress(h); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("ipv4 IHL < 5", func(t *testing.T) {
		h := make([]byte, ipv4.HeaderLen)
		h[0] = 0x44 // v=4, IHL=4
		if _, err := p.DestinationAddress(h); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("ipv4 IHL > len(header)", func(t *testing.T) {
		h := mkIPv4(5, [4]byte{1, 1, 1, 1})
		h[0] = 0x46 // claim IHL=6 (24), len=20
		if _, err := p.DestinationAddress(h); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("ipv6 too small", func(t *testing.T) {
		h := make([]byte, ipv6.HeaderLen-1)
		h[0] = 0x60
		if _, err := p.DestinationAddress(h); err == nil {
			t.Fatal("expected error")
		}
	})
}
