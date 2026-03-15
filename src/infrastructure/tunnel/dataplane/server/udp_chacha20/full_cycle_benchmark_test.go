package udp_chacha20

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"tungo/application/network/connection"
	chacha "tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/session"

	"golang.org/x/crypto/chacha20poly1305"
)

type benchmarkHandshake struct {
	id  [32]byte
	c2s []byte
	s2c []byte
}

func (h *benchmarkHandshake) Id() [32]byte              { return h.id }
func (h *benchmarkHandshake) KeyClientToServer() []byte { return append([]byte(nil), h.c2s...) }
func (h *benchmarkHandshake) KeyServerToClient() []byte { return append([]byte(nil), h.s2c...) }
func (h *benchmarkHandshake) ServerSideHandshake(connection.Transport) (int, error) {
	return 0, nil
}
func (h *benchmarkHandshake) ClientSideHandshake(connection.Transport) error { return nil }

type benchmarkPacketSink struct {
	buf []byte
}

func (s *benchmarkPacketSink) Write(p []byte) (int, error) {
	s.buf = append(s.buf[:0], p...)
	return len(p), nil
}

func benchmarkUDPCryptoPair(b *testing.B, idSeed uint64) (connection.Crypto, connection.Crypto) {
	b.Helper()

	var id [32]byte
	binary.BigEndian.PutUint64(id[24:], idSeed)

	hs := &benchmarkHandshake{
		id:  id,
		c2s: bytes.Repeat([]byte{0x11}, chacha20poly1305.KeySize),
		s2c: bytes.Repeat([]byte{0x22}, chacha20poly1305.KeySize),
	}

	builder := chacha.NewUdpSessionBuilder(chacha.NewDefaultAEADBuilder())
	clientCrypto, _, err := builder.FromHandshake(hs, false)
	if err != nil {
		b.Fatalf("build client crypto: %v", err)
	}
	serverCrypto, _, err := builder.FromHandshake(hs, true)
	if err != nil {
		b.Fatalf("build server crypto: %v", err)
	}
	return clientCrypto, serverCrypto
}

func benchmarkIPv4Packet(src, dst netip.Addr, payloadLen int) []byte {
	packet := make([]byte, 20+payloadLen)
	packet[0] = 0x45
	totalLen := len(packet)
	packet[2] = byte(totalLen >> 8)
	packet[3] = byte(totalLen)
	packet[8] = 64
	packet[9] = 17
	src4 := src.As4()
	dst4 := dst.As4()
	copy(packet[12:16], src4[:])
	copy(packet[16:20], dst4[:])
	for i := 20; i < len(packet); i++ {
		packet[i] = byte(i)
	}
	return packet
}

func benchmarkIPv4Addr(a, b, c, d byte) netip.Addr {
	return netip.AddrFrom4([4]byte{a, b, c, d})
}

func benchmarkIPv4AddrPort(a, b, c, d byte, port uint16) netip.AddrPort {
	return netip.AddrPortFrom(benchmarkIPv4Addr(a, b, c, d), port)
}

func BenchmarkFullCycleClientToServerUDP(b *testing.B) {
	clientInner := netip.MustParseAddr("10.0.0.2")
	clientOuter := netip.MustParseAddrPort("203.0.113.10:51820")
	remoteDst := netip.MustParseAddr("1.1.1.1")
	payloadSizes := []int{64, 512, 1400}

	for _, payloadSize := range payloadSizes {
		b.Run(benchmarkSizeName(payloadSize), func(b *testing.B) {
			clientCrypto, serverCrypto := benchmarkUDPCryptoPair(b, 1)
			cipherSink := &benchmarkPacketSink{}
			clientEgress := connection.NewDefaultEgress(cipherSink, clientCrypto)

			repo := session.NewDefaultRepository()
			peer := session.NewPeer(
				session.NewSessionWithAuth(serverCrypto, nil, clientInner, clientOuter, nil, nil),
				nil,
			)
			repo.Add(peer)

			handler := &TransportHandler{
				routeLookup: repo,
				addrUpdater: repo,
				dp:          newUdpDataplaneWorker(io.Discard, controlPlaneHandler{}),
			}

			packet := benchmarkIPv4Packet(clientInner, remoteDst, payloadSize)
			prefixLen := chacha.UDPRouteIDLength + chacha20poly1305.NonceSize
			sendBuf := make([]byte, prefixLen+len(packet), settings.DefaultEthernetMTU+settings.UDPChacha20Overhead)

			b.ReportAllocs()
			b.SetBytes(int64(len(packet)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				copy(sendBuf[prefixLen:], packet)
				if err := clientEgress.SendDataIP(sendBuf[:prefixLen+len(packet)]); err != nil {
					b.Fatalf("client encrypt/send: %v", err)
				}
				if err := handler.handlePacket(clientOuter, cipherSink.buf); err != nil {
					b.Fatalf("server handle packet: %v", err)
				}
			}
		})
	}
}

func benchmarkSizeName(payloadSize int) string {
	return fmt.Sprintf("%dB", payloadSize)
}

func BenchmarkFullCycleClientToServerUDPParallelPeers(b *testing.B) {
	type flow struct {
		mu     sync.Mutex
		outer  netip.AddrPort
		packet []byte
		egress connection.Egress
		sink   *benchmarkPacketSink
	}

	const payloadSize = 1400
	peerCounts := []int{1, 64, 1024}
	remoteDst := netip.MustParseAddr("1.1.1.1")

	for _, peerCount := range peerCounts {
		b.Run(fmt.Sprintf("%dpeers", peerCount), func(b *testing.B) {
			repo := session.NewDefaultRepository()
			handler := &TransportHandler{
				routeLookup: repo,
				addrUpdater: repo,
				dp:          newUdpDataplaneWorker(io.Discard, controlPlaneHandler{}),
			}

			flows := make([]flow, peerCount)
			for i := 0; i < peerCount; i++ {
				clientInner := benchmarkIPv4Addr(10, byte(i/256), byte(i%256), 10)
				clientOuter := benchmarkIPv4AddrPort(203, 0, byte(i/256), byte(i%256), uint16(20000+i))
				clientCrypto, serverCrypto := benchmarkUDPCryptoPair(b, uint64(i+1))
				sink := &benchmarkPacketSink{}
				flows[i] = flow{
					outer:  clientOuter,
					packet: benchmarkIPv4Packet(clientInner, remoteDst, payloadSize),
					egress: connection.NewDefaultEgress(sink, clientCrypto),
					sink:   sink,
				}
				repo.Add(session.NewPeer(
					session.NewSessionWithAuth(serverCrypto, nil, clientInner, clientOuter, nil, nil),
					nil,
				))
			}

			var workerSeq atomic.Uint64
			prefixLen := chacha.UDPRouteIDLength + chacha20poly1305.NonceSize

			b.ReportAllocs()
			b.SetBytes(int64(20 + payloadSize))
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				flowIdx := int(workerSeq.Add(1)-1) % len(flows)
				f := &flows[flowIdx]
				sendBuf := make([]byte, prefixLen+len(f.packet), settings.DefaultEthernetMTU+settings.UDPChacha20Overhead)

				for pb.Next() {
					f.mu.Lock()
					copy(sendBuf[prefixLen:], f.packet)
					if err := f.egress.SendDataIP(sendBuf[:prefixLen+len(f.packet)]); err != nil {
						f.mu.Unlock()
						b.Fatalf("client encrypt/send: %v", err)
					}
					if err := handler.handlePacket(f.outer, f.sink.buf); err != nil {
						f.mu.Unlock()
						b.Fatalf("server handle packet: %v", err)
					}
					f.mu.Unlock()
				}
			})
		})
	}
}
