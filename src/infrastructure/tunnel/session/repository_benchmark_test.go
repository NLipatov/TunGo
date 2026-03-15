package session

import (
	"fmt"
	"net/netip"
	"testing"
)

func benchmarkIPv4Addr(a, b, c, d byte) netip.Addr {
	return netip.AddrFrom4([4]byte{a, b, c, d})
}

func benchmarkIPv4AddrPort(a, b, c, d byte, port uint16) netip.AddrPort {
	return netip.AddrPortFrom(benchmarkIPv4Addr(a, b, c, d), port)
}

func benchmarkRepositoryExactInternal(size int) *DefaultRepository {
	repo := NewDefaultRepository().(*DefaultRepository)
	for i := 0; i < size; i++ {
		peer := NewPeer(NewSession(
			nil,
			nil,
			benchmarkIPv4Addr(10, byte(i/256), byte(i%256), 10),
			benchmarkIPv4AddrPort(203, 0, byte(i/256), byte(i%256), uint16(10000+i)),
		), nil)
		repo.Add(peer)
	}
	return repo
}

func benchmarkRepositoryAllowedHost(size int) *DefaultRepository {
	repo := NewDefaultRepository().(*DefaultRepository)
	for i := 0; i < size; i++ {
		internal := benchmarkIPv4Addr(10, byte(i/256), byte(i%256), 10)
		allowed := benchmarkIPv4Addr(172, 16, byte(i/256), byte(i%256))
		peer := NewPeer(NewSessionWithAuth(
			nil,
			nil,
			internal,
			benchmarkIPv4AddrPort(203, 0, byte(i/256), byte(i%256), uint16(10000+i)),
			nil,
			[]netip.Prefix{netip.PrefixFrom(allowed, allowed.BitLen())},
		), nil)
		repo.Add(peer)
	}
	return repo
}

func benchmarkRepositoryRouteIDs(size int) *DefaultRepository {
	repo := NewDefaultRepository().(*DefaultRepository)
	for i := 0; i < size; i++ {
		peer := NewPeer(&fakeSessionWithCrypto{
			fakeSession: fakeSession{
				internal: benchmarkIPv4Addr(10, byte(i/256), byte(i%256), 10),
				external: benchmarkIPv4AddrPort(203, 0, byte(i/256), byte(i%256), uint16(10000+i)),
			},
			crypto: &fakeRouteCrypto{id: uint64(i + 1)},
		}, nil)
		repo.Add(peer)
	}
	return repo
}

func BenchmarkDefaultRepository_FindByDestinationIP_ExactInternal(b *testing.B) {
	sizes := []int{1, 100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("%dpeers", size), func(b *testing.B) {
			repo := benchmarkRepositoryExactInternal(size)
			target := benchmarkIPv4Addr(10, byte((size-1)/256), byte((size-1)%256), 10)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				peer, err := repo.FindByDestinationIP(target)
				if err != nil || peer == nil {
					b.Fatalf("lookup failed: peer=%v err=%v", peer, err)
				}
			}
		})
	}
}

func BenchmarkDefaultRepository_FindByDestinationIP_AllowedHost(b *testing.B) {
	sizes := []int{1, 100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("%dpeers", size), func(b *testing.B) {
			repo := benchmarkRepositoryAllowedHost(size)
			target := benchmarkIPv4Addr(172, 16, byte((size-1)/256), byte((size-1)%256))

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				peer, err := repo.FindByDestinationIP(target)
				if err != nil || peer == nil {
					b.Fatalf("lookup failed: peer=%v err=%v", peer, err)
				}
			}
		})
	}
}

func BenchmarkDefaultRepository_FindByDestinationIP_Miss(b *testing.B) {
	sizes := []int{1, 100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("%dpeers", size), func(b *testing.B) {
			repo := benchmarkRepositoryExactInternal(size)
			target := benchmarkIPv4Addr(192, 0, 2, 123)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				peer, err := repo.FindByDestinationIP(target)
				if err != ErrNotFound || peer != nil {
					b.Fatalf("expected miss, got peer=%v err=%v", peer, err)
				}
			}
		})
	}
}

func BenchmarkDefaultRepository_GetByRouteID(b *testing.B) {
	sizes := []int{1, 100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("%dpeers", size), func(b *testing.B) {
			repo := benchmarkRepositoryRouteIDs(size)
			target := uint64(size)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				peer, err := repo.GetByRouteID(target)
				if err != nil || peer == nil {
					b.Fatalf("route lookup failed: peer=%v err=%v", peer, err)
				}
			}
		})
	}
}
