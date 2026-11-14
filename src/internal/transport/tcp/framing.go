package tcp

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
)

// framedConn serializes calls to Write. Concurrent reads, or adapters that do
// not support simultaneous Read and Write, still require external synchronization.
type framedConn struct {
	net.Conn
	frameCap int

	writeMu     sync.Mutex
	writeBuffer []byte

	// bufReader amortizes underlying Read syscalls: header + payload served from a single buffer refill.
	bufReader *bufio.Reader
	// pre-allocated header buffer for reads (to avoid any chance of escape/allocation)
	readHeaderBuffer [2]byte
}

func NewFramedConn(
	conn net.Conn,
	frameCap int,
) (*framedConn, error) {
	if conn == nil {
		return nil, fmt.Errorf("adapter must not be nil")
	}
	if frameCap <= 0 {
		return nil, fmt.Errorf("frame cap must be > 0")
	}
	if frameCap > math.MaxUint16 {
		return nil, fmt.Errorf("frame cap %d exceeds u16 transport cap %d", frameCap, math.MaxUint16)
	}
	return &framedConn{
		Conn:        conn,
		frameCap:    frameCap,
		writeBuffer: make([]byte, 2+frameCap),
		bufReader:   bufio.NewReader(conn),
	}, nil
}

// Write writes one u16-BE length-prefixed frame. Returns len(data) on success.
// Header and payload share one write so message transports do not send the
// length prefix as a separate message. The frame buffer is reused under writeMu.
// NOTE: On errors adapter DOES NOT drain; the caller MUST close the connection.
func (f *framedConn) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, ErrZeroLengthFrame
	}
	if len(data) > f.frameCap {
		return 0, ErrFrameCapExceeded
	}

	f.writeMu.Lock()
	defer f.writeMu.Unlock()

	frame := f.writeBuffer[:2+len(data)]
	binary.BigEndian.PutUint16(frame, uint16(len(data)))
	copy(frame[2:], data)
	if err := f.writeFull(f.Conn, frame); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (f *framedConn) writeFull(w io.Writer, p []byte) error {
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
func (f *framedConn) Read(buffer []byte) (int, error) {
	if _, err := io.ReadFull(f.bufReader, f.readHeaderBuffer[:]); err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalidLengthPrefixHeader, err)
	}
	length := int(binary.BigEndian.Uint16(f.readHeaderBuffer[:]))
	if length == 0 {
		return 0, ErrZeroLengthFrame
	}
	if length > f.frameCap {
		return 0, ErrFrameCapExceeded
	}
	if length > len(buffer) {
		return 0, io.ErrShortBuffer
	}
	if _, err := io.ReadFull(f.bufReader, buffer[:length]); err != nil {
		return 0, err
	}
	return length, nil
}
