package adapters

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"tungo/application/network/connection"
	framelimit "tungo/domain/network/ip/frame_limit"
)

// compile time check (LengthPrefixFramingAdapter must implement application.Transport)
var _ interface {
	connection.Transport
} = (*LengthPrefixFramingAdapter)(nil)

// Not safe for concurrent Read/Write without external synchronization.
type LengthPrefixFramingAdapter struct {
	adapter  connection.Transport
	frameCap framelimit.Cap
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
	return &LengthPrefixFramingAdapter{adapter: adapter, frameCap: frameCap}, nil
}

// Write writes one u16-BE length-prefixed frame. Returns len(data) on success.
// NOTE: On errors adapter DOES NOT drain; the caller MUST close the connection.
func (a *LengthPrefixFramingAdapter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, ErrZeroLengthFrame
	}
	if capErr := a.frameCap.ValidateLen(len(data)); capErr != nil {
		return 0, capErr
	}
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(data)))
	if err := a.writeFull(a.adapter, hdr[:]); err != nil {
		return 0, err
	}
	if err := a.writeFull(a.adapter, data); err != nil {
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
	var hdr [2]byte
	if _, err := io.ReadFull(a.adapter, hdr[:]); err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalidLengthPrefixHeader, err)
	}
	length := int(binary.BigEndian.Uint16(hdr[:]))
	if length == 0 {
		return 0, ErrZeroLengthFrame
	}
	if capErr := a.frameCap.ValidateLen(length); capErr != nil {
		return 0, capErr
	}
	if length > len(buffer) {
		return 0, io.ErrShortBuffer
	}
	if _, err := io.ReadFull(a.adapter, buffer[:length]); err != nil {
		return 0, err
	}
	return length, nil
}

func (a *LengthPrefixFramingAdapter) Close() error { return a.adapter.Close() }
