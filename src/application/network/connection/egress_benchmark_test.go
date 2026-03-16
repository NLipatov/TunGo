package connection

import (
	"fmt"
	"testing"
)

type egressBenchCrypto struct{}

func (egressBenchCrypto) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (egressBenchCrypto) Decrypt(b []byte) ([]byte, error) { return b, nil }

type egressBenchWriter struct{}

func (egressBenchWriter) Write(p []byte) (int, error) { return len(p), nil }

func BenchmarkDefaultEgress_SendDataIP(b *testing.B) {
	sizes := []int{64, 512, 1400}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("%dB", size), func(b *testing.B) {
			egress := NewDefaultEgress(egressBenchWriter{}, egressBenchCrypto{})
			payload := make([]byte, size)

			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = egress.SendDataIP(payload)
			}
		})
	}
}

func BenchmarkDefaultEgress_SendDataIPParallel(b *testing.B) {
	sizes := []int{64, 512, 1400}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("%dB", size), func(b *testing.B) {
			egress := NewDefaultEgress(egressBenchWriter{}, egressBenchCrypto{})
			payload := make([]byte, size)

			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_ = egress.SendDataIP(payload)
				}
			})
		})
	}
}
