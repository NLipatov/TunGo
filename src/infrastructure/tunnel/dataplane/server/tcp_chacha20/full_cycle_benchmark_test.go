package tcp_chacha20

import (
	"bytes"
	"fmt"
	"io"
	"net/netip"
	"testing"
	"tungo/application/network/connection"
	chacha "tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network/ip"
	"tungo/infrastructure/network/service_packet"
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

func benchmarkTCPCryptoPair(b *testing.B) (connection.Crypto, connection.Crypto) {
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

	builder := chacha.NewTcpSessionBuilder(chacha.NewDefaultAEADBuilder())
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

func benchmarkHandleTCPServerIngress(peer *session.Peer, tunWriter io.Writer, packet []byte) error {
	if len(packet) < chacha20poly1305.Overhead || len(packet) > settings.DefaultEthernetMTU+settings.TCPChacha20Overhead {
		return nil
	}
	if !peer.CryptoRLock() {
		return nil
	}
	pt, err := peer.Crypto().Decrypt(packet)
	peer.CryptoRUnlock()
	if err != nil {
		return err
	}
	if spType, spOk := service_packet.TryParseHeader(pt); spOk {
		switch spType {
		case service_packet.RekeyInit, service_packet.Ping:
			return nil
		}
	}
	srcIP, srcOk := ip.ExtractSourceIP(pt)
	if !srcOk || !peer.Session.IsSourceAllowed(srcIP) {
		return nil
	}
	_, err = tunWriter.Write(pt)
	return err
}

func BenchmarkFullCycleClientToServerTCP(b *testing.B) {
	clientInner := netip.MustParseAddr("10.0.0.2")
	clientOuter := netip.MustParseAddrPort("203.0.113.10:51820")
	remoteDst := netip.MustParseAddr("1.1.1.1")
	payloadSizes := []int{64, 512, 1400}

	for _, payloadSize := range payloadSizes {
		b.Run(fmt.Sprintf("%dB", payloadSize), func(b *testing.B) {
			clientCrypto, serverCrypto := benchmarkTCPCryptoPair(b)
			cipherSink := &benchmarkPacketSink{}
			clientEgress := connection.NewDefaultEgress(cipherSink, clientCrypto)
			tunSink := &benchmarkPacketSink{}

			peer := session.NewPeer(
				session.NewSession(serverCrypto, nil, clientInner, clientOuter),
				nil,
			)

			packet := benchmarkIPv4Packet(clientInner, remoteDst, payloadSize)
			sendBuf := make([]byte, epochPrefixSize+len(packet), settings.DefaultEthernetMTU+settings.TCPChacha20Overhead)

			b.ReportAllocs()
			b.SetBytes(int64(len(packet)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				copy(sendBuf[epochPrefixSize:], packet)
				if err := clientEgress.SendDataIP(sendBuf[:epochPrefixSize+len(packet)]); err != nil {
					b.Fatalf("client encrypt/send: %v", err)
				}
				if err := benchmarkHandleTCPServerIngress(peer, tunSink, cipherSink.buf); err != nil {
					b.Fatalf("server handle packet: %v", err)
				}
			}
		})
	}
}
