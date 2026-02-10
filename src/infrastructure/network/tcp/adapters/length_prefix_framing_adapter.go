package adapters

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/netip"
	"tungo/application/network/connection"
	framelimit "tungo/domain/network/ip/frame_limit"
)

// LengthPrefixFramingAdapter is not safe for concurrent Read/Write without external synchronization.
type LengthPrefixFramingAdapter struct {
	adapter  connection.Transport
	frameCap framelimit.Cap

	// bufReader amortizes underlying Read syscalls: header + payload served from a single buffer refill.
	bufReader *bufio.Reader
	// pre-allocated header buffer for reads (to avoid any chance of escape/allocation)
	readHeaderBuffer [2]byte
	// pre-allocated buffer for writes: 2-byte header + payload combined into single syscall
	writeBuf []byte
}

func NewLengthPrefixFramingAdapter(
	adapter connection.Transport,
	frameCap framelimit.Cap,
) (*LengthPrefixFramingAdapter, error) {
	if adapter == nil {
		return nil, fmt.Errorf("adapter must not be nil")
	}
	if int(frameCap) <= 0 {
		return nil, fmt.Errorf("frame cap must be > 0")
	}
	if int(frameCap) > math.MaxUint16 {
		return nil, fmt.Errorf("frame cap %d exceeds u16 transport cap %d", int(frameCap), math.MaxUint16)
	}
	return &LengthPrefixFramingAdapter{
		adapter:   adapter,
		frameCap:  frameCap,
		bufReader: bufio.NewReader(adapter),
		writeBuf:  make([]byte, 2+int(frameCap)),
	}, nil
}

// Write writes one u16-BE length-prefixed frame. Returns len(data) on success.
// Header and payload are combined into a single write to avoid double syscall.
// NOTE: On errors adapter DOES NOT drain; the caller MUST close the connection.
func (a *LengthPrefixFramingAdapter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, ErrZeroLengthFrame
	}
	if capErr := a.frameCap.ValidateLen(len(data)); capErr != nil {
		return 0, capErr
	}
	binary.BigEndian.PutUint16(a.writeBuf[:2], uint16(len(data)))
	copy(a.writeBuf[2:], data)
	if err := a.writeFull(a.adapter, a.writeBuf[:2+len(data)]); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (a *LengthPrefixFramingAdapter) writeFull(w io.Writer, p []byte) error {
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
func (a *LengthPrefixFramingAdapter) Read(buffer []byte) (int, error) {
	if _, err := io.ReadFull(a.bufReader, a.readHeaderBuffer[:]); err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalidLengthPrefixHeader, err)
	}
	length := int(binary.BigEndian.Uint16(a.readHeaderBuffer[:]))
	if length == 0 {
		return 0, ErrZeroLengthFrame
	}
	if capErr := a.frameCap.ValidateLen(length); capErr != nil {
		return 0, capErr
	}
	if length > len(buffer) {
		return 0, io.ErrShortBuffer
	}
	if _, err := io.ReadFull(a.bufReader, buffer[:length]); err != nil {
		return 0, err
	}
	return length, nil
}

func (a *LengthPrefixFramingAdapter) Close() error { return a.adapter.Close() }

// RemoteAddrPort delegates to the inner transport if it implements
// TransportWithRemoteAddr (e.g. via RemoteAddrTransport). This allows
// the Noise handshake to extract the client IP for cookie binding
// through the adapter chain.
func (a *LengthPrefixFramingAdapter) RemoteAddrPort() netip.AddrPort {
	if t, ok := a.adapter.(connection.TransportWithRemoteAddr); ok {
		return t.RemoteAddrPort()
	}
	return netip.AddrPort{}
}
