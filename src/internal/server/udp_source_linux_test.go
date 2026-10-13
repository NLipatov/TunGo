package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"syscall"
	"testing"
	"time"

	serverconfig "tungo/internal/config/server"
	udpcrypto "tungo/internal/protocol/chacha20/udp"
	"tungo/internal/protocol/keys"
	"tungo/internal/protocol/noise"
	"tungo/internal/protocol/servicepacket"
	"tungo/internal/server/session"
	serverudp "tungo/internal/server/udp"
)

// Loopback clients address secondary local IPs so that a wildcard socket's
// default reply source differs from the request destination. Linux routes
// 127.0.0.2 through loopback; CI also adds fd00::2/128 to lo for the IPv6 cases.
func TestUDPRepliesUseRequestDestination(t *testing.T) {
	otherIPv4 := netip.MustParseAddr("127.0.0.2")
	otherIPv6 := netip.MustParseAddr("fd00::2")
	deriver := &keys.DefaultKeyDeriver{}
	serverPub, serverPriv, err := deriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientPub, clientPriv, err := deriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	allowed := newAllowedPeers([]serverconfig.AllowedPeer{{PublicKey: clientPub, Enabled: true, ClientID: 1}})
	for _, tc := range []struct {
		name, network, bind string
		target              netip.Addr
	}{
		{"ipv4_wildcard", "udp4", "0.0.0.0", otherIPv4},
		{"dual_stack_ipv4", "udp", "::", otherIPv4},
		{"ipv4_wildcard_dual_stack", "udp", "0.0.0.0", otherIPv4},
		{"ipv4_specific", "udp", otherIPv4.String(), otherIPv4},
		{"ipv6_wildcard", "udp6", "::", otherIPv6},
		{"dual_stack_ipv6", "udp", "::", otherIPv6},
		{"ipv4_wildcard_dual_stack_ipv6", "udp", "0.0.0.0", otherIPv6},
		{"ipv6_specific", "udp", otherIPv6.String(), otherIPv6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.target.Is6() {
				// Local runs may lack the CI fixture. Probe it independently of
				// packet-info setup; errors in the tested path must still fail.
				probe, err := net.ListenUDP("udp6", &net.UDPAddr{IP: tc.target.AsSlice()})
				if errors.Is(err, syscall.EADDRNOTAVAIL) || errors.Is(err, syscall.EAFNOSUPPORT) {
					t.Skip("requires: ip -6 addr add fd00::2/128 dev lo nodad")
				}
				if err != nil {
					t.Fatal(err)
				}
				_ = probe.Close()
			}
			bind := netip.MustParseAddr(tc.bind)
			listener, err := net.ListenUDP(tc.network, net.UDPAddrFromAddrPort(netip.AddrPortFrom(bind, 0)))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			destination := netip.AddrPortFrom(tc.target, listener.LocalAddr().(*net.UDPAddr).AddrPort().Port())
			ctx, cancel := context.WithCancel(t.Context())
			tunReader, tunWriter := io.Pipe()
			repo := session.NewRepository()
			srv, err := serverudp.New(ctx, struct {
				io.Reader
				io.Writer
			}{tunReader, io.Discard}, listener, repo, func() *noise.IKHandshake {
				return noise.NewIKHandshakeServer(serverPub, serverPriv[:], allowed, nil, nil)
			}, netip.MustParsePrefix("10.0.1.0/24"), netip.Prefix{})
			if err != nil {
				cancel()
				_ = tunReader.Close()
				_ = tunWriter.Close()
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- srv.Run() }()
			t.Cleanup(func() {
				cancel()
				_ = tunReader.Close()
				_ = tunWriter.Close()
				_ = listener.Close()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Error("UDP server did not stop")
				}
			})
			dial := func() *net.UDPConn {
				t.Helper()
				local := net.IPv6loopback
				if tc.target.Is4() {
					local = net.IPv4(127, 0, 0, 1)
				}
				conn, err := net.DialUDP("udp", &net.UDPAddr{IP: local}, net.UDPAddrFromAddrPort(destination))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = conn.Close() })
				if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
					t.Fatal(err)
				}
				return conn
			}
			client := dial()
			handshake := noise.NewIKHandshakeClient(clientPub, clientPriv[:], serverPub)
			if err := handshake.ClientSideHandshake(client); err != nil {
				t.Fatalf("handshake with %s: %v", destination, err)
			}
			clientCrypto, _, err := udpcrypto.NewFromHandshake(handshake, false)
			if err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(time.Second)
			var peer *session.Peer
			for peer == nil {
				peer, _ = repo.GetByInternalAddrPort(netip.MustParseAddr("10.0.1.2"))
				if time.Now().After(deadline) {
					t.Fatal("client was not registered")
				}
				if peer == nil {
					time.Sleep(time.Millisecond)
				}
			}
			payload := []byte("packet sent through Peer")
			frame := make([]byte, udpcrypto.PayloadOffset+len(payload), udpcrypto.PayloadOffset+len(payload)+16)
			copy(frame[udpcrypto.PayloadOffset:], payload)
			if err := peer.Send(frame); err != nil {
				t.Fatal(err)
			}
			receive := func(conn *net.UDPConn) []byte {
				t.Helper()
				buffer := make([]byte, 1500)
				n, from, err := conn.ReadFromUDPAddrPort(buffer)
				if err != nil {
					t.Fatal(err)
				}
				if from.Addr().Unmap() != destination.Addr() || from.Port() != destination.Port() {
					t.Fatalf("reply from %s, want %s", from, destination)
				}
				plaintext, err := clientCrypto.Decrypt(buffer[:n])
				if err != nil {
					t.Fatal(err)
				}
				return plaintext
			}
			if got := receive(client); !bytes.Equal(got, payload) {
				t.Fatalf("payload = %q, want %q", got, payload)
			}
			// A valid packet from a new client port updates the destination, while
			// the server must keep the same source address for its response.
			roamed := dial()
			ping := make([]byte, udpcrypto.PayloadOffset+3, udpcrypto.PayloadOffset+3+16)
			if err := servicepacket.Encode(servicepacket.Ping, ping[udpcrypto.PayloadOffset:]); err != nil {
				t.Fatal(err)
			}
			ciphertext, err := clientCrypto.Encrypt(ping)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := roamed.Write(ciphertext); err != nil {
				t.Fatal(err)
			}
			if kind, ok := servicepacket.Parse(receive(roamed)); !ok || kind != servicepacket.Pong {
				t.Fatalf("response after roaming = %v, valid=%v", kind, ok)
			}
		})
	}
}
