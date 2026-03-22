package adapters

import (
	"net"
	"net/netip"
	appudp "tungo/application/network/udp"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
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

type batchingServerIngress struct {
	conn   *net.UDPConn
	source batchSource
	batch  []batchReadMessage
}

func NewBatchingServerIngress(conn *net.UDPConn) appudp.Ingress {
	return &batchingServerIngress{
		conn:   conn,
		source: newBatchSource(conn),
	}
}

func newBatchSource(conn *net.UDPConn) batchSource {
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok && addr.IP.To4() != nil {
		return &ipv4BatchSource{conn: ipv4.NewPacketConn(conn)}
	}
	return &ipv6BatchSource{conn: ipv6.NewPacketConn(conn)}
}

func (r *batchingServerIngress) Close() error {
	return r.conn.Close()
}

func (r *batchingServerIngress) SetReadBuffer(size int) error {
	return r.conn.SetReadBuffer(size)
}

func (r *batchingServerIngress) ReadBatch(packets []appudp.Packet) (int, error) {
	if cap(r.batch) < len(packets) {
		r.batch = make([]batchReadMessage, len(packets))
	} else {
		r.batch = r.batch[:len(packets)]
	}

	for i := range packets {
		buffer := packets[i].Data
		if cap(buffer) > 0 {
			buffer = buffer[:cap(buffer)]
		}
		r.batch[i].Buffer = buffer
		r.batch[i].N = 0
		r.batch[i].Addr = netip.AddrPort{}
		r.batch[i].Flags = 0
	}

	n, err := r.source.ReadBatch(r.batch)
	if err != nil {
		return 0, err
	}
	for i := 0; i < n; i++ {
		packets[i].Data = r.batch[i].Buffer[:r.batch[i].N]
		packets[i].Addr = r.batch[i].Addr
		packets[i].Flags = r.batch[i].Flags
	}
	return n, nil
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
