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
	appip "tungo/infrastructure/network/ip"
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

func BenchmarkFullCycleServerToClientUDP(b *testing.B) {
	clientInner := netip.MustParseAddr("10.0.0.2")
	clientOuter := netip.MustParseAddrPort("203.0.113.10:51820")
	remoteSrc := netip.MustParseAddr("1.1.1.1")
	parser := appip.NewHeaderParser()
	payloadSizes := []int{64, 512, 1400}

	for _, payloadSize := range payloadSizes {
		b.Run(fmt.Sprintf("%dB", payloadSize), func(b *testing.B) {
			clientCrypto, serverCrypto := benchmarkUDPCryptoPair(b, 1)
			cipherSink := &benchmarkPacketSink{}

			repo := session.NewDefaultRepository()
			peer := session.NewPeer(
				session.NewSessionWithAuth(serverCrypto, nil, clientInner, clientOuter, nil, nil),
				connection.NewDefaultEgress(cipherSink, serverCrypto),
			)
			repo.Add(peer)

			handler := &TransportHandler{
				writer:              io.Discard,
				cryptographyService: clientCrypto,
			}

			packet := benchmarkIPv4Packet(remoteSrc, clientInner, payloadSize)
			prefixLen := chacha.UDPRouteIDLength + chacha20poly1305.NonceSize
			sendBuf := make([]byte, prefixLen+len(packet), settings.DefaultEthernetMTU+settings.UDPChacha20Overhead)

			b.ReportAllocs()
			b.SetBytes(int64(len(packet)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				addr, err := parser.DestinationAddress(packet)
				if err != nil {
					b.Fatalf("parse destination: %v", err)
				}
				peer, err := repo.FindByDestinationIP(addr)
				if err != nil {
					b.Fatalf("find peer: %v", err)
				}
				copy(sendBuf[prefixLen:], packet)
				if err := peer.Egress().SendDataIP(sendBuf[:prefixLen+len(packet)]); err != nil {
					b.Fatalf("server encrypt/send: %v", err)
				}
				written, err := handler.handleDatagram(cipherSink.buf)
				if err != nil {
					b.Fatalf("client handle datagram: %v", err)
				}
				if written != len(packet) {
					b.Fatalf("unexpected payload length: got %d want %d", written, len(packet))
				}
			}
		})
	}
}

func BenchmarkFullCycleServerToClientUDPParallelPeers(b *testing.B) {
	type flow struct {
		mu     sync.Mutex
		packet []byte
		sink   *benchmarkPacketSink
		egress connection.Egress
		crypto connection.Crypto
	}

	const payloadSize = 1400
	peerCounts := []int{1, 64, 1024}
	remoteSrc := netip.MustParseAddr("1.1.1.1")
	parser := appip.NewHeaderParser()

	for _, peerCount := range peerCounts {
		b.Run(fmt.Sprintf("%dpeers", peerCount), func(b *testing.B) {
			repo := session.NewDefaultRepository()
			flows := make([]flow, peerCount)

			for i := 0; i < peerCount; i++ {
				clientInner := benchmarkIPv4Addr(10, byte(i/256), byte(i%256), 10)
				clientOuter := benchmarkIPv4AddrPort(203, 0, byte(i/256), byte(i%256), uint16(20000+i))
				clientCrypto, serverCrypto := benchmarkUDPCryptoPair(b, uint64(i+1))
				sink := &benchmarkPacketSink{}
				egress := connection.NewDefaultEgress(sink, serverCrypto)
				repo.Add(session.NewPeer(
					session.NewSessionWithAuth(serverCrypto, nil, clientInner, clientOuter, nil, nil),
					egress,
				))
				flows[i] = flow{
					packet: benchmarkIPv4Packet(remoteSrc, clientInner, payloadSize),
					sink:   sink,
					egress: egress,
					crypto: clientCrypto,
				}
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
				handler := &TransportHandler{
					writer:              io.Discard,
					cryptographyService: f.crypto,
				}

				for pb.Next() {
					f.mu.Lock()
					addr, err := parser.DestinationAddress(f.packet)
					if err != nil {
						f.mu.Unlock()
						b.Fatalf("parse destination: %v", err)
					}
					peer, err := repo.FindByDestinationIP(addr)
					if err != nil {
						f.mu.Unlock()
						b.Fatalf("find peer: %v", err)
					}
					copy(sendBuf[prefixLen:], f.packet)
					if err := peer.Egress().SendDataIP(sendBuf[:prefixLen+len(f.packet)]); err != nil {
						f.mu.Unlock()
						b.Fatalf("server encrypt/send: %v", err)
					}
					written, err := handler.handleDatagram(f.sink.buf)
					if err != nil {
						f.mu.Unlock()
						b.Fatalf("client handle datagram: %v", err)
					}
					if written != len(f.packet) {
						f.mu.Unlock()
						b.Fatalf("unexpected payload length: got %d want %d", written, len(f.packet))
					}
					f.mu.Unlock()
				}
			})
		})
	}
}
