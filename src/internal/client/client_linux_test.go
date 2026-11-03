package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"tungo/internal/config/settings"
	"tungo/internal/protocol/keys"
	"tungo/internal/protocol/noise"
	transport "tungo/internal/transport/tcp"
)

// This test opens a real, temporary TUN but stubs every network configuration
// command. It requires /dev/net/tun and CAP_NET_ADMIN, as in the Linux CI job.
func TestRunSessionEstablishedTunnelCleanup(t *testing.T) {
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		t.Skipf("TUN device unavailable: %v", err)
	}
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var capabilities [2]unix.CapUserData
	if err := unix.Capget(&header, &capabilities[0]); err != nil {
		t.Fatal(err)
	}
	if capabilities[0].Effective&(1<<unix.CAP_NET_ADMIN) == 0 {
		t.Skip("opening a TUN requires CAP_NET_ADMIN")
	}

	for index, end := range []string{"cancel", "server disconnect"} {
		t.Run(end, func(t *testing.T) {
			stubClientNetworkCommands(t)
			// The manager parses a route to the local server. All other ip commands
			// also use this stub, so no real addresses or routes are changed.
			if err := os.WriteFile(filepath.Join(os.Getenv("PATH"), "ip"),
				[]byte("#!/bin/sh\nprintf '127.0.0.1 dev lo\\n'\n"), 0700); err != nil {
				t.Fatal(err)
			}

			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			deriver := &keys.DefaultKeyDeriver{}
			serverPublic, serverPrivate, err := deriver.GenerateX25519KeyPair()
			if err != nil {
				t.Fatal(err)
			}
			clientPublic, clientPrivate, err := deriver.GenerateX25519KeyPair()
			if err != nil {
				t.Fatal(err)
			}
			client := newTestClient(t)
			configuration := clientTestConfiguration()
			// Keep the name distinct from configured interfaces and other test processes.
			tunName := fmt.Sprintf("tgtest%x%d", os.Getpid(), index)
			if _, err := net.InterfaceByName(tunName); err == nil {
				t.Fatalf("test interface %q already exists", tunName)
			}
			configuration.TCPSettings.TunName = tunName
			configuration.TCPSettings.Port = listener.Addr().(*net.TCPAddr).Port
			configuration.ClientPublicKey = clientPublic
			configuration.ClientPrivateKey = clientPrivate[:]
			configuration.X25519PublicKey = serverPublic
			importClientConfiguration(t, client.configurations, "server", configuration)
			if err := client.configurations.Activate("server"); err != nil {
				t.Fatal(err)
			}

			handshake := noise.NewIKHandshakeServer(serverPublic, serverPrivate[:],
				newTestAllowedPeers([]testPeer{{PublicKey: clientPublic, Enabled: true, ClientID: 1}}), nil, nil)
			type handshakeResult struct {
				conn *net.TCPConn
				err  error
			}
			established := make(chan handshakeResult, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					established <- handshakeResult{err: err}
					return
				}
				t.Cleanup(func() { _ = conn.Close() })
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				framed, err := transport.NewFramedConn(conn, settings.DefaultEthernetMTU+settings.TCPChacha20Overhead)
				if err == nil {
					_, err = handshake.ServerSideHandshake(framed)
				}
				established <- handshakeResult{conn: conn.(*net.TCPConn), err: err}
			}()

			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			stopped := make(chan struct{})
			go func() {
				done <- client.runSession(ctx)
				close(stopped)
			}()
			t.Cleanup(func() {
				cancel()
				select {
				case <-stopped:
				case <-time.After(3 * time.Second):
					t.Error("session did not stop during test cleanup")
				}
			})

			var serverConn *net.TCPConn
			select {
			case result := <-established:
				if result.err != nil {
					t.Fatalf("server handshake: %v", result.err)
				}
				serverConn = result.conn
			case err := <-done:
				t.Fatalf("session ended before handshake: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("handshake timed out")
			}

			deadline := time.NewTimer(3 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(time.Millisecond)
			defer tick.Stop()
			for !client.Ready() {
				select {
				case err := <-done:
					t.Fatalf("session ended before becoming ready: %v", err)
				case <-deadline.C:
					t.Fatal("session never became ready")
				case <-tick.C:
				}
			}
			if _, err := net.InterfaceByName(tunName); err != nil {
				t.Fatalf("ready session has no TUN: %v", err)
			}

			if end == "cancel" {
				cancel()
			} else if err := serverConn.CloseWrite(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if end == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatalf("runSession() error = %v, want cancellation", err)
				}
				if end == "server disconnect" && !errors.Is(err, io.EOF) {
					t.Fatalf("runSession() error = %v, want EOF", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("established session did not stop")
			}
			if client.Ready() {
				t.Fatal("ended session is still ready")
			}
			if client.sessionReadyAt.IsZero() {
				t.Fatal("established session did not record its ready time")
			}
			if client.tunManager != nil {
				t.Fatal("successful cleanup retained the TUN manager")
			}
			if _, err := net.InterfaceByName(tunName); err == nil {
				t.Fatal("TUN interface remains after session cleanup")
			}
			// The server only closed its write half, so EOF here proves that the
			// client also closed its transport, rather than just stopping its loop.
			_ = serverConn.SetReadDeadline(time.Now().Add(time.Second))
			if _, err := io.Copy(io.Discard, serverConn); err != nil {
				t.Fatalf("drain server connection after session cleanup: %v", err)
			}
		})
	}
}
