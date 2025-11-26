package adapters

import (
	"net/netip"
	"tungo/application/listeners"
	"tungo/application/network/connection"
	"tungo/infrastructure/network/udp/queue"
)

// RegistrationAdapter adapts registrationQueue to connection.Transport
// so that handshake implementation can use its usual Read/Write interface.
type RegistrationAdapter struct {
	conn     listeners.UdpListener
	addrPort netip.AddrPort
	queue    queue.RegistrationQueue
}

func NewRegistrationTransport(
	conn listeners.UdpListener,
	addrPort netip.AddrPort,
	queue queue.RegistrationQueue,
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
