package service

import (
	"crypto/rand"
	"testing"
)

// helper: make a slice of given size filled with random bytes.
func randBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

// --- Benchmarks for TryParseType ---

// Baseline: legacy 1-byte reset packet.
func BenchmarkTryParseType_LegacyReset(b *testing.B) {
	h := NewDefaultPacketHandler()
	pkt := []byte{byte(SessionReset)}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, ok := h.TryParseType(pkt); !ok {
			b.Fatal("unexpected not-ok")
		}
	}
}

// Legacy unknown 1-byte packet (negative path).
func BenchmarkTryParseType_LegacyUnknown(b *testing.B) {
	h := NewDefaultPacketHandler()
	pkt := []byte{0xEE}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, ok := h.TryParseType(pkt); ok {
			b.Fatal("unexpected ok")
		}
	}
}

// v1 framed reset: 0xFF 0x01 <type>.
func BenchmarkTryParseType_V1Reset(b *testing.B) {
	h := NewDefaultPacketHandler()
	pkt := []byte{Prefix, VersionV1, byte(SessionReset)}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, ok := h.TryParseType(pkt); !ok {
			b.Fatal("unexpected not-ok")
		}
	}
}

// v1 framed with wrong prefix (negative).
func BenchmarkTryParseType_V1WrongPrefix(b *testing.B) {
	h := NewDefaultPacketHandler()
	pkt := []byte{0xFE, VersionV1, byte(SessionReset)}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, ok := h.TryParseType(pkt); ok {
			b.Fatal("unexpected ok")
		}
	}
}

// Non-service realistic ciphertext payloads; should be rejected fast.
func BenchmarkTryParseType_NonServiceCiphertext(b *testing.B) {
	h := NewDefaultPacketHandler()
	sizes := []int{16, 64, 256, 1200, 1500} // 16 ~ tag/nonce floor; others ~ typical UDP payloads
	for _, n := range sizes {
		b.Run((func(n int) string { return "size=" + itoa(n) })(n), func(b *testing.B) {
			pkt := randBytes(n)
			// Ensure most are not service (rare: 3-byte packets will be generated rarely).
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, ok := h.TryParseType(pkt); ok {
					// Extremely unlikely for random data, but we guard anyway.
					b.Fatal("unexpected ok on random payload")
				}
			}
		})
	}
}

// Parallel version to simulate concurrent readers.
func BenchmarkTryParseType_ParallelMixed(b *testing.B) {
	h := NewDefaultPacketHandler()
	legacyOK := []byte{byte(SessionReset)}
	v1OK := []byte{Prefix, VersionV1, byte(SessionReset)}
	garbage := randBytes(256)

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			switch i % 3 {
			case 0:
				if _, ok := h.TryParseType(legacyOK); !ok {
					b.Fatal("legacy ok -> not ok")
				}
			case 1:
				if _, ok := h.TryParseType(v1OK); !ok {
					b.Fatal("v1 ok -> not ok")
				}
			case 2:
				if _, ok := h.TryParseType(garbage); ok {
					b.Fatal("garbage -> ok")
				}
			}
			i++
		}
	})
}

// --- Benchmarks for EncodeLegacy / EncodeV1 ---

func BenchmarkEncodeLegacy_OK(b *testing.B) {
	h := NewDefaultPacketHandler()
	buf := make([]byte, 3) // deliberately larger than needed
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		out, err := h.EncodeLegacy(SessionReset, buf)
		if err != nil || len(out) != 1 {
			b.Fatalf("err=%v len=%d", err, len(out))
		}
	}
}

func BenchmarkEncodeLegacy_ShortBuffer(b *testing.B) {
	h := NewDefaultPacketHandler()
	buf := make([]byte, 0)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := h.EncodeLegacy(SessionReset, buf); err == nil {
			b.Fatal("expected short buffer error")
		}
	}
}

func BenchmarkEncodeV1_OK(b *testing.B) {
	h := NewDefaultPacketHandler()
	buf := make([]byte, 3)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		out, err := h.EncodeV1(SessionReset, buf)
		if err != nil || len(out) != 3 {
			b.Fatalf("err=%v len=%d", err, len(out))
		}
	}
}

func BenchmarkEncodeV1_ShortBuffer(b *testing.B) {
	h := NewDefaultPacketHandler()
	buf := make([]byte, 2)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := h.EncodeV1(SessionReset, buf); err == nil {
			b.Fatal("expected short buffer error")
		}
	}
}

// --- small helpers (avoid fmt in benchmarks) ---

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		// '0' + (i % 10)
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
