package tcp

import (
	"bytes"
	"context"
	"io"
	"net/netip"
	"testing"

	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/chacha20/tcp"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
	tunnelrekey "tungo/infrastructure/tunnel/internal/rekey"
	"tungo/infrastructure/tunnel/server/internal/session"
)

type captureWriter struct {
	packet []byte
}

func (w *captureWriter) Write(packet []byte) (int, error) {
	w.packet = append(w.packet[:0], packet...)
	return len(packet), nil
}

type plaintextCrypto struct {
	plaintext []byte
}

func (c *plaintextCrypto) Encrypt(data []byte) ([]byte, error) { return data, nil }
func (c *plaintextCrypto) Decrypt([]byte) ([]byte, error)      { return c.plaintext, nil }

type epochManager struct {
	epoch uint16
}

func (m *epochManager) StageEpoch(_, _ []byte) (uint16, error) {
	m.epoch++
	return m.epoch, nil
}
func (*epochManager) PromoteSendEpoch(uint16)   {}
func (*epochManager) RetirePreviousEpoch() bool { return true }

type onePacketTun struct {
	packet []byte
	writes [][]byte
	read   bool
}

func (t *onePacketTun) Read(dst []byte) (int, error) {
	if t.read {
		return 0, io.EOF
	}
	t.read = true
	return copy(dst, t.packet), nil
}
func (t *onePacketTun) Write(packet []byte) (int, error) {
	t.writes = append(t.writes, append([]byte(nil), packet...))
	return len(packet), nil
}
func (*onePacketTun) Close() error { return nil }

func ipv4Packet(src, dst netip.Addr) []byte {
	packet := make([]byte, 20)
	packet[0] = 0x45
	src4 := src.As4()
	dst4 := dst.As4()
	copy(packet[12:16], src4[:])
	copy(packet[16:20], dst4[:])
	return packet
}

func TestServer_RunTunRoutesPacketDirectlyToPeer(t *testing.T) {
	internal := netip.MustParseAddr("10.0.0.2")
	tun := &onePacketTun{packet: ipv4Packet(netip.MustParseAddr("1.1.1.1"), internal)}
	writer := &captureWriter{}
	peer := session.NewPeer(&plaintextCrypto{}, nil, internal, netip.MustParseAddrPort("192.0.2.1:1"), writer)
	peers := session.NewRepository()
	peers.Add(peer)
	server := &Server{ctx: context.Background(), tun: tun, peers: peers}

	if err := server.runTun(); err != io.EOF {
		t.Fatalf("RunTun error = %v, want EOF", err)
	}
	if len(writer.packet) != tcp.EpochPrefixSize+len(tun.packet) {
		t.Fatalf("sent length = %d, want %d", len(writer.packet), tcp.EpochPrefixSize+len(tun.packet))
	}
	if string(writer.packet[tcp.EpochPrefixSize:]) != string(tun.packet) {
		t.Fatal("sent payload differs from TUN packet")
	}
}

func TestServer_PingDoesNotRequireRekey(t *testing.T) {
	writer := &captureWriter{}
	peer := session.NewPeer(&plaintextCrypto{}, nil, netip.Addr{}, netip.AddrPort{}, writer)
	server := &Server{}
	ping := make([]byte, 3)
	if _, err := service_packet.EncodeV1Header(service_packet.Ping, ping); err != nil {
		t.Fatal(err)
	}

	handled, err := server.handleService(peer, 0, ping)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	kind, ok := service_packet.TryParseHeader(writer.packet[tcp.EpochPrefixSize:])
	if !ok || kind != service_packet.Pong {
		t.Fatalf("response kind=%v ok=%v", kind, ok)
	}
}

func TestServer_HandleFrameDecryptsAuthorizesAndWritesTun(t *testing.T) {
	internal := netip.MustParseAddr("10.0.0.2")
	plaintext := ipv4Packet(internal, netip.MustParseAddr("1.1.1.1"))
	peer := session.NewPeer(
		&plaintextCrypto{plaintext: plaintext}, nil, internal, netip.MustParseAddrPort("192.0.2.1:1"), nil,
	)
	tun := &onePacketTun{}
	server := &Server{tun: tun}
	frame := make([]byte, tcp.EpochPrefixSize+16)

	if err := server.handleFrame(peer, frame); err != nil {
		t.Fatal(err)
	}
	if len(tun.writes) != 1 || string(tun.writes[0]) != string(plaintext) {
		t.Fatalf("TUN writes=%q", tun.writes)
	}

	peer = session.NewPeer(
		&plaintextCrypto{plaintext: ipv4Packet(netip.MustParseAddr("10.0.0.99"), netip.MustParseAddr("1.1.1.1"))},
		nil, internal, netip.MustParseAddrPort("192.0.2.1:1"), nil,
	)
	if err := server.handleFrame(peer, frame); err != nil || len(tun.writes) != 1 {
		t.Fatalf("disallowed frame: writes=%d err=%v", len(tun.writes), err)
	}
}

func TestServer_RekeySendsAckAndActivatesTCP(t *testing.T) {
	deriver := &primitives.DefaultKeyDeriver{}
	clientPub, _, err := deriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	init := make([]byte, service_packet.RekeyPacketLen)
	copy(init[3:], clientPub[:])
	if _, err := service_packet.EncodeV1Header(service_packet.RekeyInit, init); err != nil {
		t.Fatal(err)
	}
	fsm := rekey.NewStateMachine(&epochManager{}, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
	coordinator := tunnelrekey.NewServerRekeyCoordinator(fsm)
	writer := &captureWriter{}
	peer := session.NewPeer(&plaintextCrypto{}, coordinator, netip.Addr{}, netip.AddrPort{}, writer)
	server := &Server{}

	handled, err := server.handleService(peer, 0, init)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	kind, ok := service_packet.TryParseHeader(writer.packet[tcp.EpochPrefixSize:])
	if !ok || kind != service_packet.RekeyAck {
		t.Fatalf("response kind=%v ok=%v", kind, ok)
	}
	if got := coordinator.SendEpoch(); got != 1 {
		t.Fatalf("send epoch=%d, want 1", got)
	}
}
