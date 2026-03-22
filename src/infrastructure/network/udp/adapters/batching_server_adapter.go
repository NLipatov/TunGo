package adapters

import (
	"io"
	"net"
	"net/netip"
	"sync"
	"tungo/application/listeners"
	"tungo/infrastructure/settings"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	serverReadBatchSize = 32
	serverPacketBufSize = settings.DefaultEthernetMTU + settings.UDPChacha20Overhead
)

type batchReadMessage struct {
	Buffer []byte
	N      int
	Addr   netip.AddrPort
	Flags  int
}

type batchSource interface {
	ReadBatch(msgs []batchReadMessage) (int, error)
}

type batchingServerReader struct {
	conn   listeners.UdpListener
	source batchSource

	mu      sync.Mutex
	count   int
	next    int
	buffers [serverReadBatchSize][serverPacketBufSize]byte
	batch   [serverReadBatchSize]batchReadMessage
}

func NewBatchingServerAdapter(conn *net.UDPConn) listeners.UdpListener {
	r := &batchingServerReader{
		conn:   conn,
		source: newBatchSource(conn),
	}
	for i := range r.batch {
		r.batch[i].Buffer = r.buffers[i][:]
	}
	return r
}

func newBatchSource(conn *net.UDPConn) batchSource {
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok && addr.IP.To4() != nil {
		return &ipv4BatchSource{conn: ipv4.NewPacketConn(conn)}
	}
	return &ipv6BatchSource{conn: ipv6.NewPacketConn(conn)}
}

func (r *batchingServerReader) Close() error {
	return r.conn.Close()
}

func (r *batchingServerReader) ReadMsgUDPAddrPort(b, oob []byte) (n, oobn, flags int, addr netip.AddrPort, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for r.next >= r.count {
		if r.source == nil {
			return r.conn.ReadMsgUDPAddrPort(b, oob)
		}
		count, readErr := r.source.ReadBatch(r.batch[:])
		if readErr != nil {
			return 0, 0, 0, netip.AddrPort{}, readErr
		}
		if count == 0 {
			return 0, 0, 0, netip.AddrPort{}, io.ErrNoProgress
		}
		r.count = count
		r.next = 0
	}

	msg := &r.batch[r.next]
	r.next++
	n = copy(b, msg.Buffer[:msg.N])
	return n, 0, msg.Flags, msg.Addr, nil
}

func (r *batchingServerReader) SetReadBuffer(size int) error {
	return r.conn.SetReadBuffer(size)
}

func (r *batchingServerReader) SetWriteBuffer(size int) error {
	return r.conn.SetWriteBuffer(size)
}

func (r *batchingServerReader) WriteToUDPAddrPort(data []byte, addr netip.AddrPort) (int, error) {
	return r.conn.WriteToUDPAddrPort(data, addr)
}

type ipv4BatchSource struct {
	conn *ipv4.PacketConn
	msgs []ipv4.Message
}

func (r *ipv4BatchSource) ReadBatch(msgs []batchReadMessage) (int, error) {
	return readBatch(
		&r.msgs,
		msgs,
		func(batch []ipv4.Message) (int, error) {
			return r.conn.ReadBatch(batch, 0)
		},
		func(msg *ipv4.Message, buffer []byte) {
			msg.Buffers = [][]byte{buffer}
			msg.OOB = nil
			msg.Addr = nil
			msg.N = 0
			msg.NN = 0
			msg.Flags = 0
		},
		func(msg ipv4.Message) (int, net.Addr, int) {
			return msg.N, msg.Addr, msg.Flags
		},
	)
}

type ipv6BatchSource struct {
	conn *ipv6.PacketConn
	msgs []ipv6.Message
}

func (r *ipv6BatchSource) ReadBatch(msgs []batchReadMessage) (int, error) {
	return readBatch(
		&r.msgs,
		msgs,
		func(batch []ipv6.Message) (int, error) {
			return r.conn.ReadBatch(batch, 0)
		},
		func(msg *ipv6.Message, buffer []byte) {
			msg.Buffers = [][]byte{buffer}
			msg.OOB = nil
			msg.Addr = nil
			msg.N = 0
			msg.NN = 0
			msg.Flags = 0
		},
		func(msg ipv6.Message) (int, net.Addr, int) {
			return msg.N, msg.Addr, msg.Flags
		},
	)
}

func readBatch[M any](
	cache *[]M,
	msgs []batchReadMessage,
	read func([]M) (int, error),
	reset func(*M, []byte),
	decode func(M) (int, net.Addr, int),
) (int, error) {
	if cap(*cache) < len(msgs) {
		*cache = make([]M, len(msgs))
	} else {
		*cache = (*cache)[:len(msgs)]
	}

	for i := range msgs {
		reset(&(*cache)[i], msgs[i].Buffer)
	}

	n, err := read(*cache)
	if err != nil {
		return 0, err
	}
	for i := 0; i < n; i++ {
		size, addr, flags := decode((*cache)[i])
		msgs[i].N = size
		msgs[i].Addr = udpAddrPort(addr)
		msgs[i].Flags = flags
	}
	return n, nil
}

func udpAddrPort(addr net.Addr) netip.AddrPort {
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok || udpAddr == nil {
		return netip.AddrPort{}
	}
	return udpAddr.AddrPort()
}
