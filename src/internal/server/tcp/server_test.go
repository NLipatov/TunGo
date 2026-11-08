package tcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"testing"

	"tungo/internal/protocol/chacha20/rekey"
	"tungo/internal/protocol/chacha20/tcp"
	"tungo/internal/protocol/keys"
	tunnelrekey "tungo/internal/protocol/rekey"
	"tungo/internal/protocol/servicepacket"
	"tungo/internal/server/session"
)

const testRekeyPacketLen = 3 + 32

func TestServer_ServeClientReadError(t *testing.T) {
	for _, tt := range []struct {
		name     string
		data     []byte
		wantWarn bool
	}{
		{name: "clean close"},
		{name: "partial header", data: []byte{0}, wantWarn: true},
		{name: "invalid length", data: []byte{0, 0}, wantWarn: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			originalLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(originalLogger) })

			hf := &tcpRegHandshakeFactory{handshake: &tcpRegHandshake{clientID: 1}}
			cf := &tcpRegCryptoFactory{crypto: tcpRegCrypto{}}
			peers := session.NewRepository()
			server := &Server{
				ctx:   t.Context(),
				peers: peers,
				registrar: newRegistrar(
					hf.NewHandshake, cf.FromHandshake, peers,
					netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{},
				),
			}
			conn := &tcpRegConn{
				remoteAddr: &net.TCPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 12345},
				reader:     bytes.NewReader(tt.data),
			}
			server.serveClient(conn)

			if got := strings.Contains(logs.String(), "failed to read from client"); got != tt.wantWarn {
				t.Errorf("read warning = %v, want %v; logs:\n%s", got, tt.wantWarn, &logs)
			}
			if !strings.Contains(logs.String(), "client disconnected") {
				t.Errorf("missing disconnect log:\n%s", &logs)
			}
			if !conn.closed {
				t.Error("connection was not closed")
			}
			if _, err := peers.GetByInternalAddrPort(netip.MustParseAddr("10.0.0.2")); !errors.Is(err, session.ErrNotFound) {
				t.Errorf("peer was not removed: %v", err)
			}
		})
	}
}

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
	if err := servicepacket.Encode(servicepacket.Ping, ping); err != nil {
		t.Fatal(err)
	}

	handled, err := server.handleService(peer, 0, ping)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	kind, ok := servicepacket.Parse(writer.packet[tcp.EpochPrefixSize:])
	if !ok || kind != servicepacket.Pong {
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
	deriver := &keys.DefaultKeyDeriver{}
	clientPub, _, err := deriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	init := make([]byte, testRekeyPacketLen)
	copy(init[3:], clientPub[:])
	if err := servicepacket.Encode(servicepacket.RekeyInit, init); err != nil {
		t.Fatal(err)
	}
	fsm := rekey.NewStateMachine(&epochManager{}, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
	coordinator := tunnelrekey.NewServerRekeyCoordinator(fsm, nil)
	writer := &captureWriter{}
	peer := session.NewPeer(&plaintextCrypto{}, coordinator, netip.Addr{}, netip.AddrPort{}, writer)
	server := &Server{}

	handled, err := server.handleService(peer, 0, init)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	kind, ok := servicepacket.Parse(writer.packet[tcp.EpochPrefixSize:])
	if !ok || kind != servicepacket.RekeyAck {
		t.Fatalf("response kind=%v ok=%v", kind, ok)
	}
	if got := fsm.SendEpoch(); got != 1 {
		t.Fatalf("send epoch=%d, want 1", got)
	}
}

func TestServer_RekeyV2SendsNoiseAckAndActivatesTCP(t *testing.T) {
	init := append(make([]byte, 3), []byte("noise-msg1")...)
	if err := servicepacket.Encode(servicepacket.RekeyInitV2, init); err != nil {
		t.Fatal(err)
	}
	fsm := rekey.NewStateMachine(&epochManager{}, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
	coordinator := tunnelrekey.NewServerRekeyCoordinator(fsm, rekeyV2Responder{})
	writer := &captureWriter{}
	peer := session.NewPeer(&plaintextCrypto{}, coordinator, netip.Addr{}, netip.AddrPort{}, writer)
	server := &Server{}

	handled, err := server.handleService(peer, 0, init)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	response := writer.packet[tcp.EpochPrefixSize:]
	kind, ok := servicepacket.Parse(response)
	if !ok || kind != servicepacket.RekeyAckV2 || string(response[3:]) != "noise-msg2" {
		t.Fatalf("unexpected response: kind=%v ok=%v body=%q", kind, ok, response[3:])
	}
	if got := fsm.SendEpoch(); got != 1 {
		t.Fatalf("send epoch=%d, want 1", got)
	}
}
