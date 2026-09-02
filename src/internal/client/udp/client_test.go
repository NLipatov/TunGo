package udp

import (
	"context"
	"strings"
	"testing"
	"time"

	"tungo/internal/config/settings"
)

func newBlockingTestClient(readIdleFor time.Duration) (*Client, context.CancelFunc) {
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
	setActivityTrackerIdleFor(transport.readActivity, readIdleFor)

	return &Client{ctx: ctx, tun: tun, transport: transport}, cancel
}

func TestClientRun_ChecksLivenessWhileReadsAreBlocked(t *testing.T) {
	t.Parallel()

	client, cancel := newBlockingTestClient(settings.PingRestartTimeout + time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- client.Run() }()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "server unreachable") {
			t.Fatalf("Run() error = %v, want server unreachable", err)
		}
	case <-time.After(3 * livenessCheckInterval):
		t.Fatal("Run did not check liveness while reads were blocked")
	}
}

func TestClientRun_ContinuesAfterHealthyLivenessTick(t *testing.T) {
	t.Parallel()

	client, cancel := newBlockingTestClient(0)
	done := make(chan error, 1)
	go func() { done <- client.Run() }()

	select {
	case err := <-done:
		cancel()
		t.Fatalf("Run returned after a healthy liveness tick: %v", err)
	case <-time.After(livenessCheckInterval + 100*time.Millisecond):
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
}
