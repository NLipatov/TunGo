package udp

import (
	"io"
	"net"
	"net/netip"
	"sync/atomic"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type registrationQueue interface {
	ReadInto(dst []byte) (int, error)
	Close()
}

// registrationConn adapts registrationQueue to the handshake's Read/Write interface.
//
// The destination address is stored atomically so that it can be updated
// after NAT roaming without replacing the writer in the egress pipeline.
// The source IP is fixed for the lifetime of the connection, including handshake
// replies and encrypted peer traffic. Its control message is immutable.
type registrationConn struct {
	conn  *net.UDPConn
	addr  atomic.Pointer[netip.AddrPort]
	oob   []byte
	queue registrationQueue
}

func NewRegistrationConn(
	conn *net.UDPConn,
	clientAddrPort netip.AddrPort,
	serverAddr netip.Addr,
	queue registrationQueue,
) io.ReadWriteCloser {
	a := &registrationConn{
		conn:  conn,
		queue: queue,
	}
	if serverAddr.Unmap().Is4() {
		a.oob = (&ipv4.ControlMessage{Src: serverAddr.Unmap().AsSlice()}).Marshal()
	} else {
		a.oob = (&ipv6.ControlMessage{Src: serverAddr.AsSlice()}).Marshal()
	}
	a.addr.Store(&clientAddrPort)
	return a
}

func (t *registrationConn) Read(p []byte) (int, error) {
	return t.queue.ReadInto(p)
}

func (t *registrationConn) Write(p []byte) (int, error) {
	n, _, err := t.conn.WriteMsgUDPAddrPort(p, t.oob, *t.addr.Load())
	return n, err
}

func (t *registrationConn) Close() error {
	// The server owns the shared socket; the registrar owns the queue.
	return nil
}

// RemoteAddrPort returns the client's current external address.
func (t *registrationConn) RemoteAddrPort() netip.AddrPort {
	return *t.addr.Load()
}

// SetAddrPort atomically updates the destination address after NAT roaming.
func (t *registrationConn) SetAddrPort(addr netip.AddrPort) {
	t.addr.Store(&addr)
}
