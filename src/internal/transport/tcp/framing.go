package tcp

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/netip"
	"sync"
)

type remoteAddrProvider interface {
	RemoteAddrPort() netip.AddrPort
}

// framedConn serializes calls to Write. Concurrent reads, or adapters that do
// not support simultaneous Read and Write, still require external synchronization.
type framedConn struct {
	adapter  io.ReadWriteCloser
	frameCap int

	writeMu           sync.Mutex
	writeHeaderBuffer [2]byte

	// bufReader amortizes underlying Read syscalls: header + payload served from a single buffer refill.
	bufReader *bufio.Reader
	// pre-allocated header buffer for reads (to avoid any chance of escape/allocation)
	readHeaderBuffer [2]byte
}

func NewFramedConn(
	adapter io.ReadWriteCloser,
	frameCap int,
) (*framedConn, error) {
	if adapter == nil {
		return nil, fmt.Errorf("adapter must not be nil")
	}
	if frameCap <= 0 {
		return nil, fmt.Errorf("frame cap must be > 0")
	}
	if frameCap > math.MaxUint16 {
		return nil, fmt.Errorf("frame cap %d exceeds u16 transport cap %d", frameCap, math.MaxUint16)
	}
	return &framedConn{
		adapter:   adapter,
		frameCap:  frameCap,
		bufReader: bufio.NewReader(adapter),
	}, nil
}

// Write writes one u16-BE length-prefixed frame. Returns len(data) on success.
// Header and payload are written without payload copy.
// NOTE: On errors adapter DOES NOT drain; the caller MUST close the connection.
func (a *framedConn) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, ErrZeroLengthFrame
	}
	if len(data) > a.frameCap {
		return 0, ErrFrameCapExceeded
	}

	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	binary.BigEndian.PutUint16(a.writeHeaderBuffer[:], uint16(len(data)))
	if err := a.writeFull(a.adapter, a.writeHeaderBuffer[:]); err != nil {
		return 0, err
	}
	if err := a.writeFull(a.adapter, data); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (a *framedConn) writeFull(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if n > 0 {
			p = p[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

// Read reads exactly one u16-BE length-prefixed frame into buffer and returns payload size.
// NOTE: On errors adapter DOES NOT drain; the caller MUST close the connection.
func (a *framedConn) Read(buffer []byte) (int, error) {
	if _, err := io.ReadFull(a.bufReader, a.readHeaderBuffer[:]); err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalidLengthPrefixHeader, err)
	}
	length := int(binary.BigEndian.Uint16(a.readHeaderBuffer[:]))
	if length == 0 {
		return 0, ErrZeroLengthFrame
	}
	if length > a.frameCap {
		return 0, ErrFrameCapExceeded
	}
	if length > len(buffer) {
		return 0, io.ErrShortBuffer
	}
	if _, err := io.ReadFull(a.bufReader, buffer[:length]); err != nil {
		return 0, err
	}
	return length, nil
}

func (a *framedConn) Close() error { return a.adapter.Close() }

// RemoteAddrPort delegates to the inner transport if it implements
// TransportWithRemoteAddr (e.g. via remoteAddrConn). This allows
// the Noise handshake to extract the client IP for cookie binding
// through the adapter chain.
func (a *framedConn) RemoteAddrPort() netip.AddrPort {
	if t, ok := a.adapter.(remoteAddrProvider); ok {
		return t.RemoteAddrPort()
	}
	return netip.AddrPort{}
}
