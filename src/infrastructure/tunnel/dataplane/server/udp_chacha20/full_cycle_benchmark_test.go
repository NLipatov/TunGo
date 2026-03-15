package udp_chacha20

import (
	"bytes"
	"fmt"
	"io"
	"net/netip"
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

func benchmarkUDPCryptoPair(b *testing.B) (connection.Crypto, connection.Crypto) {
	b.Helper()

	hs := &benchmarkHandshake{
		id: [32]byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			9, 10, 11, 12, 13, 14, 15, 16,
			17, 18, 19, 20, 21, 22, 23, 24,
			25, 26, 27, 28, 29, 30, 31, 32,
		},
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

func BenchmarkFullCycleClientToServerUDP(b *testing.B) {
	clientInner := netip.MustParseAddr("10.0.0.2")
	clientOuter := netip.MustParseAddrPort("203.0.113.10:51820")
	remoteDst := netip.MustParseAddr("1.1.1.1")
	payloadSizes := []int{64, 512, 1400}

	for _, payloadSize := range payloadSizes {
		b.Run(benchmarkSizeName(payloadSize), func(b *testing.B) {
			clientCrypto, serverCrypto := benchmarkUDPCryptoPair(b)
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
