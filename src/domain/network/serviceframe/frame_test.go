package serviceframe

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"math/rand"
	"testing"
)

func makePayload(n int) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte(i)
	}
	return p
}

func wireFrom(v Version, k Kind, fl Flags, body []byte) []byte {
	b := make([]byte, HeaderSize+len(body))
	b[0], b[1] = MagicSF[0], MagicSF[1]
	b[2] = byte(v)
	b[3] = byte(k)
	b[4] = byte(fl)
	binary.BigEndian.PutUint16(b[5:7], uint16(len(body)))
	copy(b[HeaderSize:], body)
	return b
}

func TestNewFrame_Validate_OK(t *testing.T) {
	body := makePayload(8)
	kind := randomKind()
	f, err := NewFrame(V1, kind, 0, body)
	if err != nil {
		t.Fatalf("NewFrame returned error: %v", err)
	}
	if f.version != V1 || f.kind != kind || !bytes.Equal(f.body, body) {
		t.Fatalf("unexpected frame fields")
	}
}

func TestNewFrame_Validate_Errors(t *testing.T) {
	bodyOK := makePayload(1)

	_, err := NewFrame(V1-1, randomKind(), 0, bodyOK)
	if !errors.Is(err, ErrBadVersion) {
		t.Fatalf("expected ErrBadVersion, got %v", err)
	}

	// invalid kind: pick the first value that IsValid() == false
	var badK Kind
	for i := 0; i < 256; i++ {
		if !Kind(uint8(i)).IsValid() {
			badK = Kind(uint8(i))
			break
		}
	}
	// if all kinds are valid, skip this check
	if badK != 0 || !randomKind().IsValid() {
		_, err = NewFrame(V1, badK, 0, bodyOK)
		if !errors.Is(err, ErrBadKind) {
			t.Fatalf("expected ErrBadKind, got %v", err)
		}
	}

	tooBig := makePayload(int(MaxBody) + 1)
	_, err = NewFrame(V1, randomKind(), 0, tooBig)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("expected ErrBodyTooLarge, got %v", err)
	}
}

func TestMarshalUnmarshal_RoundTrip(t *testing.T) {
	body := makePayload(32)

	kind := randomKind()
	frame, err := NewFrame(V1, kind, 0, body)
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}
	wire, err := frame.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}

	var got Frame
	if err := got.UnmarshalBinary(wire); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}

	if got.version != V1 || got.kind != kind || got.flags != 0 {
		t.Fatalf("header mismatch after roundtrip")
	}
	if !bytes.Equal(got.body, body) {
		t.Fatalf("body mismatch after roundtrip")
	}
}

func TestUnmarshalBinary_ErrTooShort(t *testing.T) {
	var f Frame
	err := f.UnmarshalBinary([]byte{0})
	if !errors.Is(err, ErrTooShort) {
		t.Fatalf("expected ErrTooShort, got %v", err)
	}
}

func TestUnmarshalBinary_ErrBadMagic(t *testing.T) {
	data := wireFrom(V1, randomKind(), 0, makePayload(1))
	data[0], data[1] = 'X', 'Y'
	var f Frame
	err := f.UnmarshalBinary(data)
	if !errors.Is(err, ErrBadMagic) {
		t.Fatalf("expected ErrBadMagic, got %v", err)
	}
}

func TestUnmarshalBinary_ErrBadVersion(t *testing.T) {
	data := wireFrom(V1-1, randomKind(), 0, makePayload(1))
	var f Frame
	err := f.UnmarshalBinary(data)
	if !errors.Is(err, ErrBadVersion) {
		t.Fatalf("expected ErrBadVersion, got %v", err)
	}
}

func TestUnmarshalBinary_ErrBadKind(t *testing.T) {
	// find a value that IsValid()==false; if none, skip
	var badK Kind
	found := false
	for i := 0; i < 256; i++ {
		kind := randomKind()
		if !kind.IsValid() {
			badK = kind
			found = true
			break
		}
	}
	if !found {
		t.Skip("no invalid kind values (IsValid() true for all); skipping")
	}
	data := wireFrom(V1, badK, 0, makePayload(1))
	var f Frame
	err := f.UnmarshalBinary(data)
	if !errors.Is(err, ErrBadKind) {
		t.Fatalf("expected ErrBadKind, got %v", err)
	}
}

func TestUnmarshalBinary_ErrBodyTooLarge(t *testing.T) {
	body := makePayload(int(MaxBody) + 1)
	// build header manually to bypass NewFrame validation
	data := wireFrom(V1, randomKind(), 0, body)
	var f Frame
	err := f.UnmarshalBinary(data)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("expected ErrBodyTooLarge, got %v", err)
	}
}

func TestUnmarshalBinary_ErrBodyTruncated(t *testing.T) {
	body := makePayload(8)
	data := wireFrom(V1, randomKind(), 0, body)
	// cut off last bytes
	data = data[:len(data)-3]
	var f Frame
	err := f.UnmarshalBinary(data)
	if !errors.Is(err, ErrBodyTruncated) {
		t.Fatalf("expected ErrBodyTruncated, got %v", err)
	}
}

func TestUnmarshalBinary_ZeroCopyBehaviour(t *testing.T) {
	body := makePayload(4)
	data := wireFrom(V1, randomKind(), 0, body)

	var frame Frame
	if err := frame.UnmarshalBinary(data); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}

	// mutate the backing buffer and ensure frame.body sees the change
	data[HeaderSize] ^= 0xFF
	if frame.body[0] != (body[0] ^ 0xFF) {
		t.Fatalf("expected zero-copy body to reflect mutations of input buffer")
	}
}

func TestValidate_Errors(t *testing.T) {
	f := Frame{version: V1 - 1, kind: randomKind(), flags: 0, body: makePayload(1)}
	if err := f.Validate(); !errors.Is(err, ErrBadVersion) {
		t.Fatalf("expected ErrBadVersion")
	}
	f = Frame{version: V1, kind: randomKind(), flags: 0, body: makePayload(int(MaxBody) + 1)}
	if err := f.Validate(); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("expected ErrBodyTooLarge")
	}
}

func TestMarshalBinary_HeaderLayout(t *testing.T) {
	f, err := NewFrame(V1, KindMTUAck, 0x42, []byte{1, 2})
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}
	data, err := f.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}

	if data[0] != MagicSF[0] || data[1] != MagicSF[1] {
		t.Fatalf("bad magic bytes: %v", data[:2])
	}
	if got := Version(data[2]); got != V1 {
		t.Fatalf("bad version byte: %v", got)
	}
	if got := Kind(data[3]); got != KindMTUAck {
		t.Fatalf("bad kind byte: %v", got)
	}
	if got := Flags(data[4]); got != 0x42 {
		t.Fatalf("bad flags byte: %#x", got)
	}
	if got := binary.BigEndian.Uint16(data[5:7]); got != 2 {
		t.Fatalf("bad payload len: %d", got)
	}
	if !bytes.Equal(data[HeaderSize:], []byte{1, 2}) {
		t.Fatalf("bad payload content")
	}
}

func TestAccessors(t *testing.T) {
	body := makePayload(5)
	const fl = Flags(0x07)
	f, err := NewFrame(V1, KindMTUProbe, fl, body)
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}

	if f.Version() != V1 {
		t.Fatalf("Version() mismatch: got %v", f.Version())
	}
	if f.Kind() != KindMTUProbe {
		t.Fatalf("Kind() mismatch: got %v", f.Kind())
	}
	if f.Flags() != fl {
		t.Fatalf("Flags() mismatch: got %v", f.Flags())
	}
	if !bytes.Equal(f.Body(), body) {
		t.Fatalf("Body() mismatch: got %v", f.Body())
	}
}
func TestUnmarshalBinary_PreservesFlags(t *testing.T) {
	const fl = Flags(0xA5)
	wire := wireFrom(V1, KindMTUAck, fl, makePayload(3))

	var f Frame
	if err := f.UnmarshalBinary(wire); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}
	if f.Flags() != fl {
		t.Fatalf("flags mismatch: got %#x, want %#x", f.Flags(), fl)
	}
}

func TestUnmarshalBinary_EmptyBody(t *testing.T) {
	wire := wireFrom(V1, KindSessionReset, 0, nil)

	var f Frame
	if err := f.UnmarshalBinary(wire); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}
	if len(f.Body()) != 0 {
		t.Fatalf("expected empty body, got %d", len(f.Body()))
	}
}

func TestUnmarshalBinary_MaxBody_OK(t *testing.T) {
	body := makePayload(int(MaxBody))
	wire := wireFrom(V1, KindSessionReset, 0, body)

	var f Frame
	if err := f.UnmarshalBinary(wire); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}
	if !bytes.Equal(f.Body(), body) {
		t.Fatalf("body mismatch at MaxBody")
	}
}

func TestMarshalBinary_BufferReuseAndInvalidation(t *testing.T) {
	f, err := NewFrame(V1, KindSessionReset, 0, []byte{1, 2, 3})
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}

	buf1, err := f.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(1): %v", err)
	}
	p1 := &buf1[0]

	// Change the body and marshal again; the buffer should be reused and buf1 invalidated.
	f.body = []byte{9, 8}
	buf2, err := f.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(2): %v", err)
	}
	p2 := &buf2[0]

	if p1 != p2 {
		t.Fatalf("expected internal buffer reuse between MarshalBinary calls")
	}

	// buf1 should reflect new contents as well (invalidation guarantee).
	var got Frame
	if err := got.UnmarshalBinary(buf1); err != nil {
		t.Fatalf("UnmarshalBinary(buf1): %v", err)
	}
	if !bytes.Equal(got.Body(), []byte{9, 8}) {
		t.Fatalf("expected buf1 to be invalidated and reflect new body, got %v", got.Body())
	}
}

func TestMarshalBinary_ReallocateWhenCapTooSmall(t *testing.T) {
	f, err := NewFrame(V1, KindSessionReset, 0, makePayload(4))
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}
	// Force tiny buffer to take the reallocation branch.
	f.marshalBuffer = make([]byte, 0, 1)
	data, err := f.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	wantTotal := HeaderSize + 4
	if cap(f.marshalBuffer) < wantTotal {
		t.Fatalf("expected reallocated cap >= %d, got %d", wantTotal, cap(f.marshalBuffer))
	}
	if len(data) != wantTotal {
		t.Fatalf("unexpected data length: got %d, want %d", len(data), wantTotal)
	}
}

func TestMarshalBinary_ErrBadKind(t *testing.T) {
	f, err := NewFrame(V1, KindSessionReset, 0, makePayload(1))
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}
	// Corrupt kind after construction to trigger Validate() inside MarshalBinary.
	f.kind = Kind(math.MaxUint8)
	if _, err := f.MarshalBinary(); !errors.Is(err, ErrBadKind) {
		t.Fatalf("expected ErrBadKind from MarshalBinary, got %v", err)
	}
}

func TestValidate_ErrBadKind(t *testing.T) {
	f := Frame{version: V1, kind: Kind(math.MaxUint8), flags: 0, body: makePayload(1)}
	if err := f.Validate(); !errors.Is(err, ErrBadKind) {
		t.Fatalf("expected ErrBadKind, got %v", err)
	}
}

func BenchmarkMarshalBinary_Small(b *testing.B) {
	frame, _ := NewFrame(V1, randomKind(), 0, makePayload(32))

	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		_, _ = frame.MarshalBinary()
	}
}

func BenchmarkMarshalBinary_MaxBody(b *testing.B) {
	frame, _ := NewFrame(V1, randomKind(), 0, makePayload(int(MaxBody)))

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = frame.MarshalBinary()
	}
}

func BenchmarkUnmarshalBinary_Small(b *testing.B) {
	wire := wireFrom(V1, randomKind(), 0, makePayload(32))
	var frame Frame

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = frame.UnmarshalBinary(wire)
	}
}

func BenchmarkUnmarshalBinary_MaxBody(b *testing.B) {
	wire := wireFrom(V1, randomKind(), 0, makePayload(int(MaxBody)))
	var frame Frame

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = frame.UnmarshalBinary(wire)
	}
}

func BenchmarkMarshalBinary(b *testing.B) {
	frame, _ := NewFrame(V1, Kind(rand.Intn(3)), 0, makePayload(16))

	b.ReportAllocs()
	for range b.N {
		_, _ = frame.MarshalBinary()
	}
}

func BenchmarkUnmarshalBinary(b *testing.B) {
	wire := wireFrom(V1, randomKind(), 0, makePayload(16))
	var frame Frame

	b.ReportAllocs()
	for range b.N {
		_ = frame.UnmarshalBinary(wire)
	}
}

func randomKind() Kind {
	idx := rand.Intn(3)
	switch idx {
	case 1:
		return KindMTUProbe
	case 2:
		return KindMTUProbe
	default:
		return KindSessionReset
	}
}
