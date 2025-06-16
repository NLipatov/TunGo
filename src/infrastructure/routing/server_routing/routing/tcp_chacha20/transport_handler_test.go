package tcp_chacha20

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"tungo/infrastructure/settings"
)

// --- Mocks ---

type TransportHandlerMockListener struct {
	acceptC chan net.Conn
	errC    chan error
	mu      sync.Mutex
	closed  bool
}

func NewTransportHandlerMockListener() *TransportHandlerMockListener {
	return &TransportHandlerMockListener{
		acceptC: make(chan net.Conn, 1),
		errC:    make(chan error, 1),
	}
}
func (l *TransportHandlerMockListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.acceptC:
		return c, nil
	case err := <-l.errC:
		return nil, err
	}
}
func (l *TransportHandlerMockListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return nil
}
func (l *TransportHandlerMockListener) Addr() net.Addr { return &net.TCPAddr{} }

type TransportHandlerMockConn struct {
	net.Conn
	r bytes.Buffer
	w bytes.Buffer
}

func (c *TransportHandlerMockConn) Read(b []byte) (int, error)  { return c.r.Read(b) }
func (c *TransportHandlerMockConn) Write(b []byte) (int, error) { return c.w.Write(b) }
func (c *TransportHandlerMockConn) Close() error                { return nil }
func (c *TransportHandlerMockConn) RemoteAddr() net.Addr        { return &net.TCPAddr{} }

type TransportHandlerMockReadWriteCloser struct{ bytes.Buffer }

func (m *TransportHandlerMockReadWriteCloser) Close() error { return nil }

type TransportHandlerMockLogger struct {
	logs []string
}

func (l *TransportHandlerMockLogger) Printf(format string, _ ...any) {
	l.logs = append(l.logs, format)
}

type TransportHandlerMockSessionMgr struct {
	added, deleted bool
	getErr         error
}

func (m *TransportHandlerMockSessionMgr) Add(_ Session)    { m.added = true }
func (m *TransportHandlerMockSessionMgr) Delete(_ Session) { m.deleted = true }
func (m *TransportHandlerMockSessionMgr) GetByInternalIP([]byte) (Session, error) {
	return Session{}, m.getErr
}
func (m *TransportHandlerMockSessionMgr) GetByExternalIP([]byte) (Session, error) {
	return Session{}, nil
}

// --- Tests ---

func TestTransportHandler_HandleTransport_acceptFail(t *testing.T) {
	listener := NewTransportHandlerMockListener()
	logger := &TransportHandlerMockLogger{}
	ctx, cancel := context.WithCancel(context.Background())
	handler := &TransportHandler{
		ctx:            ctx,
		settings:       settings.Settings{Port: "1234"},
		writer:         &TransportHandlerMockReadWriteCloser{},
		listener:       listener,
		sessionManager: &TransportHandlerMockSessionMgr{},
		Logger:         logger,
	}

	listener.errC <- errors.New("accept fail")
	cancel()

	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()

	<-done
	if len(logger.logs) == 0 {
		t.Error("expected log for failed accept")
	}
}

func TestTransportHandler_HandleTransport_contextCancel(t *testing.T) {
	listener := NewTransportHandlerMockListener()
	logger := &TransportHandlerMockLogger{}
	ctx, cancel := context.WithCancel(context.Background())
	handler := &TransportHandler{
		ctx:            ctx,
		settings:       settings.Settings{Port: "1234"},
		writer:         &TransportHandlerMockReadWriteCloser{},
		listener:       listener,
		sessionManager: &TransportHandlerMockSessionMgr{},
		Logger:         logger,
	}
	cancel()
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	<-done
	if len(logger.logs) == 0 {
		t.Error("expected log for server listening")
	}
}

func TestTransportHandler_registerClient_handshakeFail(t *testing.T) {
	conn := &TransportHandlerMockConn{}
	writer := &TransportHandlerMockReadWriteCloser{}
	logger := &TransportHandlerMockLogger{}
	mgr := &TransportHandlerMockSessionMgr{}
	handler := &TransportHandler{
		ctx:            context.Background(),
		settings:       settings.Settings{Port: "1234"},
		writer:         writer,
		listener:       nil,
		sessionManager: mgr,
		Logger:         logger,
	}
	handler.registerClient(conn, writer, context.Background())
	if len(logger.logs) == 0 {
		t.Error("expected log for handshake fail")
	}
}

func TestTransportHandler_handleClient_sessionNotFound(t *testing.T) {
	conn := &TransportHandlerMockConn{}
	writer := &TransportHandlerMockReadWriteCloser{}
	logger := &TransportHandlerMockLogger{}
	mgr := &TransportHandlerMockSessionMgr{getErr: errors.New("no sess")}
	handler := &TransportHandler{
		ctx:            context.Background(),
		settings:       settings.Settings{Port: "1234"},
		writer:         writer,
		listener:       nil,
		sessionManager: mgr,
		Logger:         logger,
	}
	go func() {
		_, _ = conn.r.Write([]byte{0, 0, 0, 8, 1, 2, 3, 4, 5, 6, 7, 8})
	}()
	handler.handleClient(context.Background(), Session{conn: conn}, writer)
	if len(logger.logs) == 0 {
		t.Error("expected log for session not found")
	}
}
