package udp

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"tungo/internal/config/settings"
	"tungo/internal/protocol/chacha20/rekey"
)

func TestNew_SendsTunPacketsOverUDPConn(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("ListenUDP() error = %v", err)
	}
	defer func() { _ = server.Close() }()
	conn, err := net.DialUDP("udp", nil, server.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("DialUDP() error = %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	vpnAddr := netip.MustParseAddr("10.0.0.2")
	packet := testIPv4Pkt(vpnAddr)
	sent := false
	tun := struct {
		*fakeReader
		*fakeWriter
	}{
		fakeReader: &fakeReader{readFunc: func(p []byte) (int, error) {
			if !sent {
				sent = true
				return copy(p, packet), nil
			}
			<-ctx.Done()
			return 0, ctx.Err()
		}},
		fakeWriter: &fakeWriter{},
	}
	ctrl := rekey.NewStateMachine(dummyEpochManager{}, []byte("c2s"), []byte("s2c"))
	client := New(
		ctx,
		conn,
		tun,
		&tunhandlerTestRakeCrypto{prefix: []byte("e:")},
		newTestRekeyCoordinator(ctrl),
		map[netip.Addr]struct{}{vpnAddr: {}},
	)

	done := make(chan error, 1)
	go func() { done <- client.Run() }()

	if err := server.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	buf := make([]byte, 1500)
	n, err := server.Read(buf)
	if err != nil {
		t.Fatalf("server Read() error = %v", err)
	}
	got := buf[:n]
	if wantLen := len("e:") + udpPayloadOffset + len(packet); len(got) != wantLen ||
		!bytes.HasPrefix(got, []byte("e:")) || !bytes.HasSuffix(got, packet) {
		t.Fatalf("server received %x, want encrypted header and payload %x", got, packet)
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func newBlockingTestClient() (*Client, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	blockedRead := func([]byte) (int, error) {
		<-ctx.Done()
		return 0, ctx.Err()
	}

	tun := newTunHandler(
		ctx,
		&fakeReader{readFunc: blockedRead},
		nil,
		nil,
		nil,
	)
	transport := newTransportHandler(
		ctx,
		&fakeReader{readFunc: blockedRead},
		&fakeWriter{},
		&thTestCrypto{},
		nil,
		nil,
	)
	return &Client{ctx: ctx, tun: tun, transport: transport}, cancel
}

func TestClientRun_ChecksLivenessWhileReadsAreBlocked(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		client, cancel := newBlockingTestClient()
		defer cancel()

		done := make(chan error, 1)
		go func() { done <- client.Run() }()

		select {
		case err := <-done:
			if err == nil || !strings.Contains(err.Error(), "server unreachable") {
				t.Fatalf("Run() error = %v, want server unreachable", err)
			}
		case <-time.After(settings.PingRestartTimeout + 2*livenessCheckInterval):
			t.Fatal("Run did not check liveness while reads were blocked")
		}
	})
}

func TestClientRun_ContinuesAfterHealthyLivenessTick(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		client, cancel := newBlockingTestClient()
		done := make(chan error, 1)
		go func() { done <- client.Run() }()

		time.Sleep(livenessCheckInterval + 100*time.Millisecond)
		select {
		case err := <-done:
			cancel()
			t.Fatalf("Run returned after a healthy liveness tick: %v", err)
		default:
		}

		cancel()
		synctest.Wait()
		select {
		case <-done: //nolint: errcheck
		default:
			t.Fatal("Run did not stop after context cancellation")
		}
	})
}
