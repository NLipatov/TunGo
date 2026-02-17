package adapters

import (
	"net/netip"
	"sync/atomic"
	"tungo/application/listeners"
	"tungo/application/network/connection"
)

type registrationQueue interface {
	ReadInto(dst []byte) (int, error)
	Close()
}

// RegistrationAdapter adapts registrationQueue to connection.Transport
// so that handshake implementation can use its usual Read/Write interface.
//
// The destination address is stored atomically so that it can be updated
// after NAT roaming without replacing the writer in the egress pipeline.
type RegistrationAdapter struct {
	conn  listeners.UdpListener
	addr  atomic.Pointer[netip.AddrPort]
	queue registrationQueue
}

func NewRegistrationTransport(
	conn listeners.UdpListener,
	addrPort netip.AddrPort,
	queue registrationQueue,
) connection.Transport {
	a := &RegistrationAdapter{
		conn:  conn,
		queue: queue,
	}
	a.addr.Store(&addrPort)
	return a
}

func (t *RegistrationAdapter) Read(p []byte) (int, error) {
	return t.queue.ReadInto(p)
}

func (t *RegistrationAdapter) Write(p []byte) (int, error) {
	return t.conn.WriteToUDPAddrPort(p, *t.addr.Load())
}

func (t *RegistrationAdapter) Close() error {
	// Do not close the shared UDP socket; its lifecycle is controlled by
	// TransportHandler. The queue is closed by removeRegistrationQueue.
	return nil
}

// RemoteAddrPort returns the client's current external address.
func (t *RegistrationAdapter) RemoteAddrPort() netip.AddrPort {
	return *t.addr.Load()
}

// SetAddrPort atomically updates the destination address after NAT roaming.
func (t *RegistrationAdapter) SetAddrPort(addr netip.AddrPort) {
	t.addr.Store(&addr)
}
