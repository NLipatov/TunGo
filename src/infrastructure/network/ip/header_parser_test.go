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

func BenchmarkDestinationAddress_IPv4_Min(b *testing.B) {
	var p HeaderParser
	h := makeIPv4Header([4]byte{198, 51, 100, 42}, ipv4.HeaderLen)
	b.ReportAllocs()
	b.SetBytes(int64(len(h)))
	for i := 0; i < b.N; i++ {
		_, _ = p.DestinationAddress(h)
	}
}

func BenchmarkDestinationAddress_IPv4_WithOptions(b *testing.B) {
	var p HeaderParser
	// IHL=6 (24 bytes) to simulate presence of options.
	h := makeIPv4Header([4]byte{203, 0, 113, 7}, 24)
	b.ReportAllocs()
	b.SetBytes(int64(len(h)))
	for i := 0; i < b.N; i++ {
		_, _ = p.DestinationAddress(h)
	}
}

func BenchmarkDestinationAddress_IPv6(b *testing.B) {
	var p HeaderParser
	var d [16]byte
	// 2001:db8::1
	d[0], d[1] = 0x20, 0x01
	d[2], d[3] = 0x0d, 0xb8
	d[15] = 0x01
	h := makeIPv6Header(d)
	b.ReportAllocs()
	b.SetBytes(int64(len(h)))
	for i := 0; i < b.N; i++ {
		_, _ = p.DestinationAddress(h)
	}
}

func BenchmarkDestinationAddress_Mixed(b *testing.B) {
	var p HeaderParser
	headers := [][]byte{
		makeIPv4Header([4]byte{192, 0, 2, 1}, ipv4.HeaderLen),
		makeIPv6Header([16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x2a}),
		makeIPv4Header([4]byte{198, 51, 100, 200}, ipv4.HeaderLen),
		makeIPv4Header([4]byte{203, 0, 113, 255}, 24),
		makeIPv6Header([16]byte{0x26, 0x07, 0xf8, 0xb0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x88, 0x88}),
	}
	b.ReportAllocs()
	total := 0
	for _, h := range headers {
		total += len(h)
	}
	b.SetBytes(int64(total / len(headers)))
	for i := 0; i < b.N; i++ {
		h := headers[i%len(headers)]
		_, _ = p.DestinationAddress(h)
	}
}

func BenchmarkDestinationAddress_RunParallel(b *testing.B) {
	var p HeaderParser
	v4 := makeIPv4Header([4]byte{203, 0, 113, 9}, ipv4.HeaderLen)
	v6 := makeIPv6Header([16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x1})
	b.ReportAllocs()
	b.SetBytes(int64((len(v4) + len(v6)) / 2))
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			var h []byte
			if i&1 == 0 {
				h = v4
			} else {
				h = v6
			}
			i++
			_, _ = p.DestinationAddress(h)
		}
	})
}

func makeIPv4Header(dest [4]byte, ihl int) []byte {
	if ihl < ipv4.HeaderLen {
		ihl = ipv4.HeaderLen
	}
	if ihl%4 != 0 {
		ihl += 4 - (ihl % 4)
	}
	h := make([]byte, ihl)
	// Version=4 (0b0100), IHL in 32-bit words.
	h[0] = 0x40 | byte(ihl/4)
	// Destination address at bytes 16..19 (independent of options).
	h[16], h[17], h[18], h[19] = dest[0], dest[1], dest[2], dest[3]
	return h
}

func makeIPv6Header(dest [16]byte) []byte {
	h := make([]byte, ipv6.HeaderLen)
	// Version=6 (0b0110)
	h[0] = 0x60
	copy(h[24:40], dest[:])
	return h
}
