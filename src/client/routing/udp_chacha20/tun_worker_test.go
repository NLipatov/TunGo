package udp_chacha20

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"tungo/crypto/chacha20"
	"tungo/network"
)

// fakeTun implements the TunAdapter interface.
type fakeTun struct {
	data       []byte
	readCalled bool
	mu         sync.Mutex
	written    [][]byte // capture writes for verification
}

func (f *fakeTun) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Return preset data only once; then simulate error to break the loop.
	if f.readCalled {
		return 0, errors.New("read error")
	}
	f.readCalled = true
	copy(p, f.data)
	return len(f.data), nil
}

func (f *fakeTun) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Capture written data.
	f.written = append(f.written, p)
	return len(p), nil
}

func (f *fakeTun) Close() error {
	return nil
}

// fakeRouter implements a minimal UDPRouter.
type fakeRouter struct {
	tun network.TunAdapter
}

// fakeSession implements chacha20.UdpSession.
type fakeSession struct{}

func (s *fakeSession) Encrypt(plaintext []byte) ([]byte, error) {
	// For testing, prepend "enc:" to simulate encryption.
	return append([]byte("enc:"), plaintext...), nil
}

func (s *fakeSession) Decrypt(ciphertext []byte) ([]byte, error) {
	prefix := []byte("enc:")
	if len(ciphertext) < len(prefix) || string(ciphertext[:len(prefix)]) != string(prefix) {
		return nil, errors.New("decryption failed")
	}
	return ciphertext[len(prefix):], nil
}

// fakeEncoder implements chacha20.UDPEncoder.
type fakeEncoder struct{}

func (e *fakeEncoder) Encode(payload []byte, nonce *chacha20.Nonce) (*chacha20.UDPPacket, error) {
	return &chacha20.UDPPacket{
		Payload: payload,
		Nonce:   nonce,
	}, nil
}

func (e *fakeEncoder) Decode(data []byte) (*chacha20.UDPPacket, error) {
	return &chacha20.UDPPacket{
		Payload: data,
		Nonce:   &chacha20.Nonce{},
	}, nil
}

// TestBuild verifies that Build fails if required dependencies are missing.
func TestUdpTunWorker_BuildError(t *testing.T) {
	worker := newUdpTunWorker()
	// No dependencies set.
	_, err := worker.Build()
	if err == nil {
		t.Fatal("expected error when dependencies are missing")
	}
}

// TestHandlePacketsFromTun simulates reading from TUN and sending encrypted data via UDP.
func TestHandlePacketsFromTun(t *testing.T) {
	// Prepare fake TUN that returns a test packet.
	testPacket := []byte("test packet")
	ftun := &fakeTun{data: testPacket}
	router := &fakeRouter{tun: ftun}

	// Create a UDP connection for testing.
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("address resolution error: %v", err)
	}
	udpConn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("failed to create UDP connection: %v", err)
	}
	defer func(udpConn *net.UDPConn) {
		_ = udpConn.Close()
	}(udpConn)

	worker := newUdpTunWorker().
		UseRouter(&UDPRouter{tun: router.tun}).
		UseConn(udpConn).
		UseSession(&fakeSession{}).
		UseEncoder(&fakeEncoder{})
	builtWorker, err := worker.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Use a context that cancels shortly.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	reconnectTriggered := false
	triggerReconnect := func() {
		reconnectTriggered = true
		cancel()
	}

	// Run HandlePacketsFromTun in a goroutine.
	go func() {
		_ = builtWorker.HandlePacketsFromTun(ctx, triggerReconnect)
	}()

	// Wait until context is done.
	<-ctx.Done()

	// Verify that reconnect was triggered due to read error.
	if !reconnectTriggered {
		t.Error("expected triggerReconnect to be called")
	}
}

// TestHandlePacketsFromConn simulates receiving an encrypted UDP packet and writing the decrypted result to TUN.
func TestHandlePacketsFromConn(t *testing.T) {
	// Create a UDP connection.
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udpConn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func(udpConn *net.UDPConn) {
		_ = udpConn.Close()
	}(udpConn)

	// Prepare fake TUN to capture written data.
	ftun := &fakeTun{}
	router := &fakeRouter{tun: ftun}

	// Use the fake session.
	sess := &fakeSession{}

	worker := newUdpTunWorker().
		UseRouter(&UDPRouter{tun: router.tun}).
		UseConn(udpConn).
		UseSession(sess).
		UseEncoder(&fakeEncoder{})
	builtWorker, err := worker.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Prepare a fake encrypted packet.
	plaintext := []byte("hello tun")
	encrypted, err := sess.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}

	// Write the encrypted packet into the UDP connection.
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = udpConn.WriteToUDP(encrypted, udpConn.LocalAddr().(*net.UDPAddr))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	// Run HandlePacketsFromConn in a goroutine.
	go func() {
		_ = builtWorker.HandlePacketsFromConn(ctx, cancel)
	}()

	// Wait until context is done.
	<-ctx.Done()

	// Verify that the decrypted packet was written to fake TUN.
	ftun.mu.Lock()
	defer ftun.mu.Unlock()
	if len(ftun.written) == 0 {
		t.Error("expected data to be written to TUN, but none found")
	} else if string(ftun.written[0]) != string(plaintext) {
		t.Errorf("expected TUN write %q, got %q", plaintext, ftun.written[0])
	}
}
