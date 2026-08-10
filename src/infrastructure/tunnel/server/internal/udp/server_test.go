package udp

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/netip"
	"testing"

	"tungo/infrastructure/cryptography/chacha20/rekey"
	udpcrypto "tungo/infrastructure/cryptography/chacha20/udp"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
	tunnelrekey "tungo/infrastructure/tunnel/internal/rekey"
	"tungo/infrastructure/tunnel/server/internal/session"
)

const testRekeyPacketLen = 3 + 32

type passthroughCrypto struct {
	plaintext []byte
	decrypts  int
	routeID   uint64
}

func (c *passthroughCrypto) Encrypt(data []byte) ([]byte, error) { return data, nil }
func (c *passthroughCrypto) Decrypt([]byte) ([]byte, error) {
	c.decrypts++
	return c.plaintext, nil
}
func (c *passthroughCrypto) RouteID() uint64 { return c.routeID }

type captureWriter struct {
	packet []byte
}

func (w *captureWriter) Write(packet []byte) (int, error) {
	w.packet = append(w.packet[:0], packet...)
	return len(packet), nil
}

type epochManager struct {
	epoch uint16
}

type rekeyV2Responder struct{}

func (rekeyV2Responder) RespondRekeyV2([]byte, []byte) ([]byte, []byte, []byte, error) {
	return []byte("noise-msg2"), bytes.Repeat([]byte{3}, 32), bytes.Repeat([]byte{4}, 32), nil
}

func (m *epochManager) StageEpoch(_, _ []byte) (uint16, error) {
	m.epoch++
	return m.epoch, nil
}
func (*epochManager) PromoteSendEpoch(uint16)   {}
func (*epochManager) RetirePreviousEpoch() bool { return true }

type testTun struct {
	packet []byte
	writes [][]byte
	read   bool
}

func (t *testTun) Read(dst []byte) (int, error) {
	if t.read {
		return 0, io.EOF
	}
	t.read = true
	return copy(dst, t.packet), nil
}
func (t *testTun) Write(packet []byte) (int, error) {
	t.writes = append(t.writes, append([]byte(nil), packet...))
	return len(packet), nil
}
func (*testTun) Close() error { return nil }

func ipv4Packet(src, dst netip.Addr) []byte {
	packet := make([]byte, 20)
	packet[0] = 0x45
	src4 := src.As4()
	dst4 := dst.As4()
	copy(packet[12:16], src4[:])
	copy(packet[16:20], dst4[:])
	return packet
}

func TestServer_HandleEstablishedWritesPlaintextToTun(t *testing.T) {
	internal := netip.MustParseAddr("10.0.0.2")
	plaintext := ipv4Packet(internal, netip.MustParseAddr("1.1.1.1"))
	crypto := &passthroughCrypto{plaintext: plaintext}
	peer := session.NewPeer(crypto, nil, internal, netip.MustParseAddrPort("192.0.2.1:1"), nil)
	tun := &testTun{}
	server := &Server{tun: tun, peers: session.NewRepository()}
	frame := make([]byte, udpcrypto.EpochOffset+2)
	binary.BigEndian.PutUint16(frame[udpcrypto.EpochOffset:], 7)

	if err := server.handleEstablished(peer.ExternalAddrPort(), peer, frame); err != nil {
		t.Fatal(err)
	}
	if len(tun.writes) != 1 || string(tun.writes[0]) != string(plaintext) {
		t.Fatalf("TUN writes = %q", tun.writes)
	}
}

func TestServer_HandleDatagramLooksUpPeerByRouteID(t *testing.T) {
	internal := netip.MustParseAddr("10.0.0.2")
	external := netip.MustParseAddrPort("192.0.2.1:51820")
	const routeID uint64 = 0x0102030405060708
	crypto := &passthroughCrypto{
		plaintext: ipv4Packet(internal, netip.MustParseAddr("1.1.1.1")),
		routeID:   routeID,
	}
	peer := session.NewPeer(crypto, nil, internal, external, nil)
	peers := session.NewRepository()
	peers.Add(peer)
	tun := &testTun{}
	server := &Server{tun: tun, peers: peers}
	frame := make([]byte, udpcrypto.MinPacketSize)
	binary.BigEndian.PutUint64(frame[:udpcrypto.RouteIDLength], routeID)
	roamed := netip.MustParseAddrPort("192.0.2.2:51820")

	if err := server.handleDatagram(roamed, frame); err != nil {
		t.Fatal(err)
	}
	if crypto.decrypts != 1 || len(tun.writes) != 1 {
		t.Fatalf("known route ID: decrypts=%d writes=%d", crypto.decrypts, len(tun.writes))
	}
	if peer.ExternalAddrPort() != roamed {
		t.Fatalf("external address = %v, want %v", peer.ExternalAddrPort(), roamed)
	}
	binary.BigEndian.PutUint64(frame[:udpcrypto.RouteIDLength], routeID+1)
	if err := server.handleDatagram(roamed, frame); err != nil {
		t.Fatal(err)
	}
	if crypto.decrypts != 1 {
		t.Fatalf("unknown route ID must not trial-decrypt peers: decrypts=%d", crypto.decrypts)
	}
}

func TestServer_RunTunRoutesPacketDirectlyToPeer(t *testing.T) {
	internal := netip.MustParseAddr("10.0.0.2")
	tun := &testTun{packet: ipv4Packet(netip.MustParseAddr("1.1.1.1"), internal)}
	writer := &captureWriter{}
	peer := session.NewPeer(&passthroughCrypto{}, nil, internal, netip.MustParseAddrPort("192.0.2.1:1"), writer)
	peers := session.NewRepository()
	peers.Add(peer)
	server := &Server{ctx: context.Background(), tun: tun, peers: peers}

	if err := server.runTun(); err != io.EOF {
		t.Fatalf("RunTun error = %v, want EOF", err)
	}
	if len(writer.packet) != udpPayloadOffset+len(tun.packet) {
		t.Fatalf("sent length = %d, want %d", len(writer.packet), udpPayloadOffset+len(tun.packet))
	}
	if string(writer.packet[udpPayloadOffset:]) != string(tun.packet) {
		t.Fatal("sent payload differs from TUN packet")
	}
}

func TestServer_PingDoesNotRequireRekey(t *testing.T) {
	writer := &captureWriter{}
	peer := session.NewPeer(&passthroughCrypto{}, nil, netip.Addr{}, netip.AddrPort{}, writer)
	server := &Server{}
	ping := make([]byte, 3)
	if err := service_packet.Encode(service_packet.Ping, ping); err != nil {
		t.Fatal(err)
	}

	handled, err := server.handleService(peer, 0, ping)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	kind, ok := service_packet.Parse(writer.packet[udpPayloadOffset:])
	if !ok || kind != service_packet.Pong {
		t.Fatalf("response kind=%v ok=%v", kind, ok)
	}
}

func TestServer_RekeySendsAckWithoutPrematureUDPActivation(t *testing.T) {
	deriver := &primitives.DefaultKeyDeriver{}
	clientPub, _, err := deriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	init := make([]byte, testRekeyPacketLen)
	copy(init[3:], clientPub[:])
	if err := service_packet.Encode(service_packet.RekeyInit, init); err != nil {
		t.Fatal(err)
	}
	fsm := rekey.NewStateMachine(&epochManager{}, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
	coordinator := tunnelrekey.NewServerRekeyCoordinator(fsm, nil)
	writer := &captureWriter{}
	peer := session.NewPeer(&passthroughCrypto{}, coordinator, netip.Addr{}, netip.AddrPort{}, writer)
	server := &Server{}

	handled, err := server.handleService(peer, 0, init)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	kind, ok := service_packet.Parse(writer.packet[udpPayloadOffset:])
	if !ok || kind != service_packet.RekeyAck {
		t.Fatalf("response kind=%v ok=%v", kind, ok)
	}
	if got := fsm.SendEpoch(); got != 0 {
		t.Fatalf("send epoch=%d, want old epoch 0 until authenticated peer traffic", got)
	}
}

func TestServer_RekeyV2SendsNoiseAckWithoutPrematureUDPActivation(t *testing.T) {
	init := append(make([]byte, 3), []byte("noise-msg1")...)
	if err := service_packet.Encode(service_packet.RekeyInitV2, init); err != nil {
		t.Fatal(err)
	}
	fsm := rekey.NewStateMachine(&epochManager{}, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
	coordinator := tunnelrekey.NewServerRekeyCoordinator(fsm, rekeyV2Responder{})
	writer := &captureWriter{}
	peer := session.NewPeer(&passthroughCrypto{}, coordinator, netip.Addr{}, netip.AddrPort{}, writer)
	server := &Server{}

	handled, err := server.handleService(peer, 0, init)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	response := writer.packet[udpPayloadOffset:]
	kind, ok := service_packet.Parse(response)
	if !ok || kind != service_packet.RekeyAckV2 || string(response[3:]) != "noise-msg2" {
		t.Fatalf("unexpected response: kind=%v ok=%v body=%q", kind, ok, response[3:])
	}
	if got := fsm.SendEpoch(); got != 0 {
		t.Fatalf("send epoch=%d, want old epoch 0 until authenticated peer traffic", got)
	}
}
