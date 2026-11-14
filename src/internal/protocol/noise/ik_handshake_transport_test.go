package noise

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
	tcptransport "tungo/internal/transport/tcp"
	udptransport "tungo/internal/transport/udp"
)

type cookieCaptureConn struct {
	net.Conn
	peer    net.Addr
	written bytes.Buffer
}

func (c *cookieCaptureConn) RemoteAddr() net.Addr        { return c.peer }
func (c *cookieCaptureConn) Write(p []byte) (int, error) { return c.written.Write(p) }

func TestIKHandshake_CookieBoundToTransportAddress(t *testing.T) {
	for _, endpoint := range []string{"198.51.100.5:1234", "[2001:db8::5]:1234", "[::ffff:198.51.100.5]:1234"} {
		t.Run(endpoint, func(t *testing.T) {
			peer := netip.MustParseAddrPort(endpoint)
			raw := &cookieCaptureConn{peer: net.TCPAddrFromAddrPort(peer)}
			framed, err := tcptransport.NewFramedConn(tcptransport.WithReadDeadline(raw, time.Second), 2048)
			if err != nil {
				t.Fatal(err)
			}
			checkTransportCookie(t, framed, peer.Addr().Unmap(), func() []byte {
				data := raw.written.Bytes()
				if len(data) < 2 || int(binary.BigEndian.Uint16(data[:2])) != len(data)-2 {
					t.Fatalf("invalid cookie frame: %d bytes", len(data))
				}
				return data[2:]
			})
		})
	}
	t.Run("udp", func(t *testing.T) {
		server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = server.Close() }()
		client, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = client.Close() }()
		peer := client.LocalAddr().(*net.UDPAddr).AddrPort()
		local := server.LocalAddr().(*net.UDPAddr).AddrPort().Addr()
		adapter := udptransport.NewRegistrationConn(server, peer, local, nil)
		checkTransportCookie(t, adapter, peer.Addr().Unmap(), func() []byte {
			if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			buffer := make([]byte, 2048)
			n, _, err := client.ReadFromUDPAddrPort(buffer)
			if err != nil {
				t.Fatal(err)
			}
			return buffer[:n]
		})
	})
}

func checkTransportCookie(t *testing.T, conn HandshakeTransport, wantIP netip.Addr, readReply func() []byte) {
	t.Helper()
	manager, err := NewCookieManager()
	if err != nil {
		t.Fatal(err)
	}
	load := NewLoadMonitor(1)
	load.handshakesPerSecond.Store(2)
	handshake := &IKHandshake{serverPubKey: bytes.Repeat([]byte{1}, 32), cookieManager: manager, loadMonitor: load}
	message := make([]byte, MinTotalSize)
	if err := handshake.enforceCookieIfNeeded(conn, message); !errors.Is(err, ErrCookieRequired) {
		t.Fatalf("cookie challenge result = %v", err)
	}
	cookie, err := DecryptCookieReply(readReply(), ExtractClientEphemeral(message), handshake.serverPubKey)
	if err != nil {
		t.Fatal(err)
	}
	if !manager.ValidateCookie(wantIP, cookie) {
		t.Fatalf("cookie not bound to %s", wantIP)
	}
	if manager.ValidateCookie(netip.MustParseAddr("203.0.113.99"), cookie) {
		t.Fatal("cookie accepted for unrelated IP")
	}
	validMessage, err := AppendMACs(make([]byte, MinMsg1Size), handshake.serverPubKey, cookie)
	if err != nil {
		t.Fatal(err)
	}
	if err := handshake.enforceCookieIfNeeded(conn, validMessage); err != nil {
		t.Fatalf("valid cookie retry rejected: %v", err)
	}
}
