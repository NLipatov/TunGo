package udp_chacha20_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
	"tungo/client/routing/udp_chacha20"

	"tungo/crypto/chacha20"
	"tungo/settings"
)

type fakeConnection struct {
	conn *net.UDPConn
	err  error
}

func (f *fakeConnection) Establish() (*net.UDPConn, error) {
	return f.conn, f.err
}

type fakeSecretExchanger struct {
	session *chacha20.UdpSession
	err     error
}

func (f *fakeSecretExchanger) Exchange(ctx context.Context, conn *net.UDPConn) (*chacha20.UdpSession, error) {
	return f.session, f.err
}

func createTestUDPConn(t *testing.T) *net.UDPConn {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestConnector_Connect_Success(t *testing.T) {
	udpConn := createTestUDPConn(t)
	defer func(udpConn *net.UDPConn) {
		_ = udpConn.Close()
	}(udpConn)

	var dummyID [32]byte
	sendKey := []byte("0123456789abcdef0123456789abcdef")
	recvKey := []byte("fedcba9876543210fedcba9876543210")
	session, err := chacha20.NewUdpSession(dummyID, sendKey, recvKey, false)
	if err != nil {
		t.Fatalf("failed to create dummy session: %v", err)
	}
	session.UseNonceRingBuffer(1024)

	fakeConn := &fakeConnection{conn: udpConn, err: nil}
	fakeExchanger := &fakeSecretExchanger{session: session, err: nil}

	connSettings := settings.ConnectionSettings{
		ConnectionIP:  "127.0.0.1",
		Port:          "12345",
		DialTimeoutMs: 5000,
	}

	connector := udp_chacha20.NewSecureConnection(connSettings, fakeConn, fakeExchanger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, sess, err := connector.Establish(ctx)
	if err != nil {
		t.Fatalf("Establish failed: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
	if sess == nil {
		t.Fatal("expected non-nil session")
	}
}

func TestConnector_Connect_FailDial(t *testing.T) {
	fakeConn := &fakeConnection{conn: nil, err: errors.New("dial error")}
	fakeExchanger := &fakeSecretExchanger{}

	connSettings := settings.ConnectionSettings{
		ConnectionIP:  "127.0.0.1",
		Port:          "12345",
		DialTimeoutMs: 5000,
	}
	connector := udp_chacha20.NewSecureConnection(connSettings, fakeConn, fakeExchanger)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, err := connector.Establish(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestConnector_Connect_FailHandshake(t *testing.T) {
	udpConn := createTestUDPConn(t)
	defer func(udpConn *net.UDPConn) {
		_ = udpConn.Close()
	}(udpConn)

	fakeConn := &fakeConnection{conn: udpConn, err: nil}
	fakeExchanger := &fakeSecretExchanger{session: nil, err: errors.New("handshake error")}

	connSettings := settings.ConnectionSettings{
		ConnectionIP:  "127.0.0.1",
		Port:          "12345",
		DialTimeoutMs: 5000,
	}
	connector := udp_chacha20.NewSecureConnection(connSettings, fakeConn, fakeExchanger)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, err := connector.Establish(ctx)
	if err == nil {
		t.Fatal("expected handshake error, got nil")
	}
}
