package adapters

import (
	"net/netip"
	"tungo/application/listeners"
	"tungo/application/network/connection"
)

type registrationQueue interface {
	ReadInto(dst []byte) (int, error)
	Close()
}

// RegistrationAdapter adapts registrationQueue to connection.Transport
// so that handshake implementation can use its usual Read/Write interface.
type RegistrationAdapter struct {
	conn     listeners.UdpListener
	addrPort netip.AddrPort
	queue    registrationQueue
}

func NewRegistrationTransport(
	conn listeners.UdpListener,
	addrPort netip.AddrPort,
	queue registrationQueue,
) connection.Transport {
	return &RegistrationAdapter{
		conn:     conn,
		addrPort: addrPort,
		queue:    queue,
	}
}

func (t *RegistrationAdapter) Read(p []byte) (int, error) {
	return t.queue.ReadInto(p)
}

func (t *RegistrationAdapter) Write(p []byte) (int, error) {
	return t.conn.WriteToUDPAddrPort(p, t.addrPort)
}

func (t *RegistrationAdapter) Close() error {
	// Do not close the shared UDP socket; its lifecycle is controlled by
	// TransportHandler. The queue is closed by removeRegistrationQueue.
	return nil
}

// RemoteAddrPort returns the client's external address for cookie IP binding.
func (t *RegistrationAdapter) RemoteAddrPort() netip.AddrPort {
	return t.addrPort
}
