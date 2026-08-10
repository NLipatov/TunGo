package servicepacket

import (
	"errors"
	"io"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		packet   []byte
		wantType HeaderType
		wantOK   bool
	}{
		{name: "empty", packet: nil},
		{name: "short", packet: []byte{Prefix, VersionV1}},
		{name: "wrong prefix", packet: []byte{0, VersionV1, byte(Ping)}},
		{name: "wrong version", packet: []byte{Prefix, 2, byte(Ping)}},
		{
			name:     "header without body",
			packet:   []byte{Prefix, VersionV1, byte(Ping)},
			wantType: Ping,
			wantOK:   true,
		},
		{
			name:     "header with arbitrary body",
			packet:   append([]byte{Prefix, VersionV1, byte(RekeyInit)}, make([]byte, 256)...),
			wantType: RekeyInit,
			wantOK:   true,
		},
		{
			name:     "unknown type is a valid v1 header",
			packet:   []byte{Prefix, VersionV1, 0xFF},
			wantType: HeaderType(0xFF),
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotOK := Parse(tt.packet)
			if gotType != tt.wantType || gotOK != tt.wantOK {
				t.Fatalf("Parse() = (%v, %v), want (%v, %v)", gotType, gotOK, tt.wantType, tt.wantOK)
			}
		})
	}
}

func TestEncode(t *testing.T) {
	body := []byte{1, 2, 3, 4}
	packet := append(make([]byte, 3), body...)

	if err := Encode(HeaderType(0xFF), packet); err != nil {
		t.Fatal(err)
	}
	if packet[0] != Prefix || packet[1] != VersionV1 || packet[2] != 0xFF {
		t.Fatalf("invalid encoded header: %v", packet[:3])
	}
	if got := packet[3:]; string(got) != string(body) {
		t.Fatalf("Encode changed body: got %v, want %v", got, body)
	}
}

func TestEncodeShortBuffer(t *testing.T) {
	if err := Encode(Ping, make([]byte, 2)); !errors.Is(err, io.ErrShortBuffer) {
		t.Fatalf("Encode() error = %v, want %v", err, io.ErrShortBuffer)
	}
}
