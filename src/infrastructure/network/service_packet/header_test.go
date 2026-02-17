package service_packet

import (
	"errors"
	"io"
	"testing"
)

// -------------------- TryParseHeader --------------------

func TestTryParseHeader(t *testing.T) {
	tests := []struct {
		name     string
		pkt      []byte
		wantType HeaderType
		wantOK   bool
	}{
		{
			name:     "single byte packet returns unknown",
			pkt:      []byte{0x01},
			wantType: Unknown,
			wantOK:   false,
		},
		{
			name:     "v1 header invalid prefix",
			pkt:      []byte{0x00, VersionV1, byte(Ping)},
			wantType: Unknown,
			wantOK:   false,
		},
		{
			name:     "v1 header invalid version",
			pkt:      []byte{Prefix, 0xFF, byte(Ping)},
			wantType: Unknown,
			wantOK:   false,
		},
		{
			name:     "v1 header unknown type",
			pkt:      []byte{Prefix, VersionV1, 0xFF},
			wantType: Unknown,
			wantOK:   false,
		},
		{
			name:     "v1 rekey init packet",
			pkt:      makeRekeyPacket(RekeyInit),
			wantType: RekeyInit,
			wantOK:   true,
		},
		{
			name:     "v1 rekey ack packet",
			pkt:      makeRekeyPacket(RekeyAck),
			wantType: RekeyAck,
			wantOK:   true,
		},
		{
			name:     "v1 rekey packet invalid prefix",
			pkt:      makeInvalidRekeyPacket(0x00, VersionV1, RekeyInit),
			wantType: Unknown,
			wantOK:   false,
		},
		{
			name:     "v1 rekey packet invalid version",
			pkt:      makeInvalidRekeyPacket(Prefix, 0xFF, RekeyInit),
			wantType: Unknown,
			wantOK:   false,
		},
		{
			name:     "v1 rekey packet unknown type",
			pkt:      makeRekeyPacket(0xFF),
			wantType: Unknown,
			wantOK:   false,
		},
		{
			name:     "v1 header ping",
			pkt:      []byte{Prefix, VersionV1, byte(Ping)},
			wantType: Ping,
			wantOK:   true,
		},
		{
			name:     "v1 header pong",
			pkt:      []byte{Prefix, VersionV1, byte(Pong)},
			wantType: Pong,
			wantOK:   true,
		},
		{
			name:     "unsupported packet length",
			pkt:      []byte{Prefix, VersionV1},
			wantType: Unknown,
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotOK := TryParseHeader(tt.pkt)
			if gotType != tt.wantType || gotOK != tt.wantOK {
				t.Fatalf(
					"TryParseHeader() = (%v, %v), want (%v, %v)",
					gotType, gotOK,
					tt.wantType, tt.wantOK,
				)
			}
		})
	}
}

// -------------------- EncodeV1Header --------------------

func TestEncodeV1Header(t *testing.T) {
	tests := []struct {
		name     string
		header   HeaderType
		dstSize  int
		wantErr  error
		wantSize int
	}{
		{
			name:     "encode v1 rekey init",
			header:   RekeyInit,
			dstSize:  RekeyPacketLen,
			wantSize: RekeyPacketLen,
		},
		{
			name:    "encode v1 rekey init short buffer",
			header:  RekeyInit,
			dstSize: RekeyPacketLen - 1,
			wantErr: io.ErrShortBuffer,
		},
		{
			name:     "encode v1 rekey ack",
			header:   RekeyAck,
			dstSize:  RekeyPacketLen,
			wantSize: RekeyPacketLen,
		},
		{
			name:    "encode v1 rekey ack short buffer",
			header:  RekeyAck,
			dstSize: RekeyPacketLen - 1,
			wantErr: io.ErrShortBuffer,
		},
		{
			name:     "encode v1 ping",
			header:   Ping,
			dstSize:  3,
			wantSize: 3,
		},
		{
			name:     "encode v1 pong",
			header:   Pong,
			dstSize:  3,
			wantSize: 3,
		},
		{
			name:    "encode v1 ping short buffer",
			header:  Ping,
			dstSize: 2,
			wantErr: io.ErrShortBuffer,
		},
		{
			name:    "encode v1 invalid header",
			header:  Unknown,
			dstSize: RekeyPacketLen,
			wantErr: ErrInvalidHeader,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := make([]byte, tt.dstSize)
			out, err := EncodeV1Header(tt.header, dst)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr == nil {
				if len(out) != tt.wantSize {
					t.Fatalf("unexpected output size: %d", len(out))
				}
				if out[0] != Prefix || out[1] != VersionV1 || out[2] != byte(tt.header) {
					t.Fatalf("invalid header encoding: %v", out[:3])
				}
			}
		})
	}
}

// -------------------- helpers --------------------

func makeRekeyPacket(typ HeaderType) []byte {
	pkt := make([]byte, RekeyPacketLen)
	pkt[0] = Prefix
	pkt[1] = VersionV1
	pkt[2] = byte(typ)
	return pkt
}

func makeInvalidRekeyPacket(prefix, version byte, typ HeaderType) []byte {
	pkt := make([]byte, RekeyPacketLen)
	pkt[0] = prefix
	pkt[1] = version
	pkt[2] = byte(typ)
	return pkt
}
