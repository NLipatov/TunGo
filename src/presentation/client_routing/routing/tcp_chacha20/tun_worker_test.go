package tcp_chacha20

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/settings"
)

// fakeTun implements the network.TunAdapter interface.
type fakeTun struct {
	readData    []byte
	readErr     error
	written     [][]byte
	closeCalled bool
	mu          sync.Mutex
}

func (f *fakeTun) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.readData) == 0 {
		// Instead of immediately erroring, wait a bit (simulate blocking) then return an error.
		time.Sleep(50 * time.Millisecond)
		return 0, errors.New("no data")
	}
	n := copy(p, f.readData)
	// Do not clear f.readData so that subsequent reads can also return the data.
	return n, nil
}

func (f *fakeTun) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.written = append(f.written, p)
	return len(p), nil
}

func (f *fakeTun) Close() error {
	f.closeCalled = true
	return nil
}

// fakeTunConfigurator implements a minimal TunConfigurator.
type fakeTunConfigurator struct {
	tun          application.TunDevice
	deconfigured bool
}

func (f *fakeTunConfigurator) Configure(_ settings.ConnectionSettings) (application.TunDevice, error) {
	return f.tun, nil
}

func (f *fakeTunConfigurator) Deconfigure(_ settings.ConnectionSettings) {
	f.deconfigured = true
}

// fakeTCPConn implements net.Conn. It uses an internal buffer for reads and captures writes.
type fakeTCPConn struct {
	readBuf     *bytes.Buffer
	written     [][]byte
	closeCalled bool
	mu          sync.Mutex
}

func newFakeTCPConn(initialData []byte) *fakeTCPConn {
	return &fakeTCPConn{
		readBuf: bytes.NewBuffer(initialData),
	}
}

func (f *fakeTCPConn) Read(b []byte) (int, error) {
	return f.readBuf.Read(b)
}

func (f *fakeTCPConn) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Capture written data.
	cp := make([]byte, len(b))
	copy(cp, b)
	f.written = append(f.written, cp)
	return len(b), nil
}

func (f *fakeTCPConn) Close() error {
	f.closeCalled = true
	return nil
}

func (f *fakeTCPConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

func (f *fakeTCPConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

func (f *fakeTCPConn) SetDeadline(_ time.Time) error      { return nil }
func (f *fakeTCPConn) SetReadDeadline(_ time.Time) error  { return nil }
func (f *fakeTCPConn) SetWriteDeadline(_ time.Time) error { return nil }

type fakeCryptographyService struct{}

func (s *fakeCryptographyService) Encrypt(plaintext []byte) ([]byte, error) {
	return append([]byte("enc:"), plaintext...), nil
}

func (s *fakeCryptographyService) Decrypt(ciphertext []byte) ([]byte, error) {
	prefix := []byte("enc:")
	if len(ciphertext) < len(prefix) || string(ciphertext[:len(prefix)]) != string(prefix) {
		return nil, errors.New("decryption failed")
	}
	return ciphertext[len(prefix):], nil
}

// TestTcpTunWorker_HandlePacketsFromTun simulates reading from TUN, encrypting, encoding, and writing to TCP.
func TestTcpTunWorker_HandlePacketsFromTun(t *testing.T) {
	// Prepare fake TUN to return test data.
	testData := []byte("hello tun")
	tun := &fakeTun{
		readData: testData,
	}
	// Create a fake TCP connection to capture writes.
	conn := newFakeTCPConn(nil)

	// Create a fake cryptographyService.
	sess := &fakeCryptographyService{}

	// Use DefaultTCPEncoder.
	encoder := &chacha20.DefaultTCPEncoder{}

	// Create a dummy TCPRouter that holds the fake TUN.
	router := &TCPRouter{
		TunConfigurator: &fakeTunConfigurator{tun: tun},
		Settings:        settings.ConnectionSettings{},
	}
	// Ensure router.tun is set using the configurator.
	router.tun, _ = router.TunConfigurator.Configure(router.Settings)

	worker, err := newTcpTunWorker().
		UseRouter(router).
		UseConn(conn).
		UseCryptographyService(sess).
		UseEncoder(encoder).
		Build()
	if err != nil {
		t.Fatalf("failed to build tcpTunWorker: %v", err)
	}

	// Use a cancelable context.
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	triggerReconnect := func() { cancel() }

	// Run the worker in a separate goroutine.
	go func() {
		_ = worker.HandleTun(ctx, triggerReconnect)
	}()

	// Wait for context cancellation.
	<-ctx.Done()

	// Verify that data was written to the fake TCP connection.
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.written) == 0 {
		t.Error("expected data to be written to TCP connection, but none found")
	} else {
		// The fake cryptographyService adds "enc:" and encoder prepends a 4-byte length.
		// Verify the length prefix.
		writtenData := conn.written[0]
		if len(writtenData) < 4 {
			t.Errorf("written data too short: %v", writtenData)
		}
		length := binary.BigEndian.Uint32(writtenData[:4])
		if int(length) != len(writtenData)-4 {
			t.Errorf("length prefix mismatch: expected %d, got %d", len(writtenData)-4, length)
		}
	}
}

// TestTcpTunWorker_HandlePacketsFromConn simulates reading a TCP-encoded packet, decrypting it, and writing to TUN.
// В TestTcpTunWorker_HandlePacketsFromConn создаём пакет с префиксом длины
func TestTcpTunWorker_HandlePacketsFromConn(t *testing.T) {
	// Prepare fake decrypted data.
	plainData := []byte("hello from TCP")
	sess := &fakeCryptographyService{}
	encryptedPayload, err := sess.Encrypt(plainData)
	if err != nil {
		t.Fatalf("failed to encrypt test data: %v", err)
	}
	// Encode the encrypted data using DefaultTCPEncoder.
	packetLen := len(encryptedPayload)
	encodedPacket := make([]byte, 4+packetLen)
	copy(encodedPacket[4:], encryptedPayload)
	encoder := &chacha20.DefaultTCPEncoder{}
	if err = encoder.Encode(encodedPacket); err != nil {
		t.Fatalf("failed to encode packet: %v", err)
	}
	// Create a buffer that simulates the TCP connection read stream.
	// The worker will first read 4 bytes (length) then the rest.
	var buf bytes.Buffer
	buf.Write(encodedPacket)

	fakeConn := newFakeTCPConn(buf.Bytes())
	fakeTun := &fakeTun{}

	router := &TCPRouter{
		TunConfigurator: &fakeTunConfigurator{tun: fakeTun},
		Settings:        settings.ConnectionSettings{},
	}
	router.tun, _ = router.TunConfigurator.Configure(router.Settings)

	worker, err := newTcpTunWorker().
		UseRouter(router).
		UseConn(fakeConn).
		UseCryptographyService(sess).
		UseEncoder(encoder).
		Build()
	if err != nil {
		t.Fatalf("failed to build tcpTunWorker: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	go func() {
		_ = worker.HandleConn(ctx, cancel)
	}()

	<-ctx.Done()

	fakeTun.mu.Lock()
	defer fakeTun.mu.Unlock()
	if len(fakeTun.written) == 0 {
		t.Error("expected data to be written to TUN, but none found")
	} else if string(fakeTun.written[0]) != string(plainData) {
		t.Errorf("expected TUN write %q, got %q", plainData, fakeTun.written[0])
	}
}
