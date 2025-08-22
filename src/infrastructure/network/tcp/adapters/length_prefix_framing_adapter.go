package adapters

import (
	"encoding/binary"
	"io"
	"tungo/application"
)

type LengthPrefixFramingAdapter struct {
	adapter  application.ConnectionAdapter
	frameCap int
}

func NewLengthPrefixFramingAdapter(
	under application.ConnectionAdapter,
	frameCap uint16,
) *LengthPrefixFramingAdapter {
	fCap := int(frameCap)
	if fCap <= 0 {
		fCap = 1
	}
	return &LengthPrefixFramingAdapter{
		adapter:  under,
		frameCap: fCap,
	}
}

// Write writes one u16-BE length-prefixed frame. Returns len(data) on success.
func (a *LengthPrefixFramingAdapter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, ErrZeroLengthFrame
	}
	if len(data) > a.frameCap {
		return 0, ErrFrameCapExceeded
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
func (a *LengthPrefixFramingAdapter) Read(buffer []byte) (int, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(a.adapter, hdr[:]); err != nil {
		return 0, ErrInvalidLengthPrefixHeader
	}
	length := int(binary.BigEndian.Uint16(hdr[:]))

	if length == 0 {
		return 0, ErrZeroLengthFrame
	}
	if length > a.frameCap {
		return 0, ErrFrameCapExceeded
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
