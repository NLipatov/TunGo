package serviceframe

import (
	"encoding/binary"
)

// Frame encodes/decodes service frames for single-threaded use.
// Concurrency: NOT safe by design.
// MarshalBinary reuses an internal buffer; the returned slice will be overwritten
// by the next MarshalBinary call. UnmarshalBinary zero-copies Body from input.
type Frame struct {
	version Version
	kind    Kind
	flags   Flags
	body    []byte

	// marshalBuffer is used for zero-alloc marshalling operations
	marshalBuffer []byte
}

func NewDefaultFrame() *Frame {
	return &Frame{
		marshalBuffer: make([]byte, 0, HeaderSize+MaxBody),
	}
}

func NewFrame(
	version Version,
	kind Kind,
	flags Flags,
	payload []byte,
) (*Frame, error) {
	frame := Frame{
		version:       version,
		kind:          kind,
		flags:         flags,
		body:          payload,
		marshalBuffer: make([]byte, 0, MaxBody+HeaderSize),
	}
	return &frame, frame.Validate()
}

func (f *Frame) Validate() error {
	if !f.version.IsValid() {
		return ErrBadVersion
	}
	if !f.kind.IsValid() {
		return ErrBadKind
	}
	if len(f.body) > MaxBody {
		return ErrBodyTooLarge
	}
	return nil
}

func (f *Frame) Version() Version {
	return f.version
}

func (f *Frame) Kind() Kind {
	return f.kind
}

func (f *Frame) Flags() Flags {
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
	total := HeaderSize + len(f.body)
	if cap(f.marshalBuffer) < total {
		f.marshalBuffer = make([]byte, 0, total)
	}
	data := f.marshalBuffer[:HeaderSize+len(f.body)]
	copy(data[:2], MagicSF[:])
	data[2] = byte(f.version)
	data[3] = byte(f.kind)
	data[4] = byte(f.flags)
	binary.BigEndian.PutUint16(data[5:7], bodyLen)
	copy(data[HeaderSize:], f.body)
	return data, nil
}

// UnmarshalBinary unmarshals a frame from data.
// NOTE: Body is a subslice of data (zero-copy). Mutating data will mutate Body.
func (f *Frame) UnmarshalBinary(data []byte) error {
	if len(data) < HeaderSize {
		return ErrTooShort
	}
	if data[0] != MagicSF[0] || data[1] != MagicSF[1] {
		return ErrBadMagic
	}
	// parse frame field values
	version := Version(data[2])
	kind := Kind(data[3])
	flags := Flags(data[4])
	payloadLen := binary.BigEndian.Uint16(data[5:7])
	if !version.IsValid() {
		return ErrBadVersion
	}
	if !kind.IsValid() {
		return ErrBadKind
	}
	if int(payloadLen) > MaxBody {
		return ErrBodyTooLarge
	}
	if len(data) < HeaderSize+int(payloadLen) {
		return ErrBodyTruncated
	}
	// set frame field values
	f.version = version
	f.kind = kind
	f.flags = flags
	// zero-copy body
	f.body = data[HeaderSize : HeaderSize+int(payloadLen)]
	return nil
}
