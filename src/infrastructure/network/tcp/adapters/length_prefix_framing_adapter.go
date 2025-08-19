package adapters

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"tungo/application"
	"tungo/infrastructure/network"
)

type LengthPrefixFramingAdapter struct {
	adapter application.ConnectionAdapter
	maxLen  int // protocol payload limit (test-injectable)
}

func NewLengthPrefixFramingAdapter(under application.ConnectionAdapter) application.ConnectionAdapter {
	return &LengthPrefixFramingAdapter{
		adapter: under,
		maxLen:  network.MaxPacketLengthBytes,
	}
}

// NewTcpAdapterWithLimit is for tests and special cases.
func NewTcpAdapterWithLimit(under application.ConnectionAdapter, limit int) *LengthPrefixFramingAdapter {
	return &LengthPrefixFramingAdapter{
		adapter: under,
		maxLen:  limit,
	}
}

// Write writes one u16-BE length-prefixed frame. Returns len(data) on success.
func (a *LengthPrefixFramingAdapter) Write(data []byte) (int, error) {
	// zero frames are not allowed by Read method
	if len(data) == 0 {
		return 0, fmt.Errorf("invalid data length: 0")
	}
	// Check protocol limit first to make this branch reachable regardless of the u16 ceiling.
	if len(data) > a.maxLen {
		return 0, fmt.Errorf("frame too large: %d > %d (protocol limit)", len(data), a.maxLen)
	}
	// Then the physical u16 prefix bound.
	if len(data) > math.MaxUint16 {
		return 0, fmt.Errorf("frame too large for u16 prefix: %d > %d", len(data), math.MaxUint16)
	}

	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(data)))
	if err := a.writeFull(a.adapter, hdr[:]); err != nil {
		return 0, fmt.Errorf("write length prefix: %w", err)
	}
	if err := a.writeFull(a.adapter, data); err != nil {
		return 0, fmt.Errorf("write payload: %w", err)
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
// If the buffer is too small (io.ErrShortBuffer) or the frame exceeds protocol limit,
// the payload is drained to keep the stream aligned.
func (a *LengthPrefixFramingAdapter) Read(buffer []byte) (int, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(a.adapter, hdr[:2]); err != nil {
		return 0, fmt.Errorf("read length prefix: %w", err)
	}
	length := int(binary.BigEndian.Uint16(hdr[:2]))

	if length == 0 {
		return 0, fmt.Errorf("invalid frame length: 0")
	}
	if length > a.maxLen {
		// Drain to keep the stream aligned; caller can retry with a bigger buffer.
		_ = a.drainN(a.adapter, length) // best-effort
		return 0, fmt.Errorf("frame length exceeds protocol limit: %d > %d", length, a.maxLen)
	}
	if length > len(buffer) {
		// Drain to keep the stream aligned; caller can retry with a bigger buffer.
		_ = a.drainN(a.adapter, length) // best-effort
		return 0, io.ErrShortBuffer
	}

	if _, err := io.ReadFull(a.adapter, buffer[:length]); err != nil {
		return 0, fmt.Errorf("read payload: %w", err)
	}
	return length, nil
}

// drainN discards exactly n bytes from r; used to keep stream in sync on short buffer.
func (a *LengthPrefixFramingAdapter) drainN(r io.Reader, n int) error {
	const chunk = 4096
	var trash [chunk]byte
	for n > 0 {
		t := n
		if t > chunk {
			t = chunk
		}
		if _, err := io.ReadFull(r, trash[:t]); err != nil {
			return err
		}
		n -= t
	}
	return nil
}

func (a *LengthPrefixFramingAdapter) Close() error { return a.adapter.Close() }
