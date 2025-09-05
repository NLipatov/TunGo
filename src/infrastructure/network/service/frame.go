package service

import (
	"encoding/binary"
	domain "tungo/domain/network/serviceframe"
)

// Frame encodes/decodes service frames for single-threaded use.
// Concurrency: NOT safe by design.
// MarshalBinary reuses an internal buffer; the returned slice will be overwritten
// by the next MarshalBinary call. UnmarshalBinary zero-copies Body from input.
type Frame struct {
	version domain.Version
	kind    domain.Kind
	flags   domain.Flags
	body    []byte

	// marshalBuffer is used for zero-alloc marshalling operations
	marshalBuffer []byte
}

func NewFrame(
	version domain.Version,
	kind domain.Kind,
	flags domain.Flags,
	payload []byte,
) (Frame, error) {
	frame := Frame{
		version:       version,
		kind:          kind,
		flags:         flags,
		body:          payload,
		marshalBuffer: make([]byte, 0, domain.MaxBody+domain.HeaderSize),
	}
	return frame, frame.Validate()
}

func (f *Frame) Validate() error {
	if !f.version.IsValid() {
		return domain.ErrBadVersion
	}
	if !f.kind.IsValid() {
		return domain.ErrBadKind
	}
	if len(f.body) > domain.MaxBody {
		return domain.ErrBodyTooLarge
	}
	return nil
}

func (f *Frame) Version() domain.Version {
	return f.version
}

func (f *Frame) Kind() domain.Kind {
	return f.kind
}

func (f *Frame) Flags() domain.Flags {
	return f.flags
}

func (f *Frame) Body() []byte {
	return f.body
}

// MarshalBinary marshals the frame into an internal reusable buffer.
// NOTE: the returned slice is invalidated by the next MarshalBinary call.
func (f *Frame) MarshalBinary() ([]byte, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	bodyLen := uint16(len(f.body))

	// reallocate marshal buffer if current is too small
	total := domain.HeaderSize + len(f.body)
	if cap(f.marshalBuffer) < total {
		f.marshalBuffer = make([]byte, 0, total)
	}
	data := f.marshalBuffer[:domain.HeaderSize+len(f.body)]
	copy(data[:2], domain.MagicSF[:])
	data[2] = byte(f.version)
	data[3] = byte(f.kind)
	data[4] = byte(f.flags)
	binary.BigEndian.PutUint16(data[5:7], bodyLen)
	copy(data[domain.HeaderSize:], f.body)
	return data, nil
}

// UnmarshalBinary unmarshals a frame from data.
// NOTE: Body is a subslice of data (zero-copy). Mutating data will mutate Body.
func (f *Frame) UnmarshalBinary(data []byte) error {
	if len(data) < domain.HeaderSize {
		return domain.ErrTooShort
	}
	if data[0] != domain.MagicSF[0] || data[1] != domain.MagicSF[1] {
		return domain.ErrBadMagic
	}
	// parse frame field values
	version := domain.Version(data[2])
	kind := domain.Kind(data[3])
	flags := domain.Flags(data[4])
	payloadLen := binary.BigEndian.Uint16(data[5:7])
	if !version.IsValid() {
		return domain.ErrBadVersion
	}
	if !kind.IsValid() {
		return domain.ErrBadKind
	}
	if int(payloadLen) > domain.MaxBody {
		return domain.ErrBodyTooLarge
	}
	if len(data) < domain.HeaderSize+int(payloadLen) {
		return domain.ErrBodyTruncated
	}
	// set frame field values
	f.version = version
	f.kind = kind
	f.flags = flags
	// zero-copy body
	f.body = data[domain.HeaderSize : domain.HeaderSize+int(payloadLen)]
	return nil
}
