package pipes

import (
	"bytes"
	"testing"

	"tungo/crypto/chacha20"
)

type fakePipe struct {
	buf bytes.Buffer
}

func (fp *fakePipe) Pass(data []byte) error {
	_, err := fp.buf.Write(data)
	return err
}

func TestTcpEncodingPipe_Success(t *testing.T) {
	input := []byte("hello")

	encoder := &chacha20.DefaultTCPEncoder{}
	expectedPacket, err := encoder.Encode(input)
	if err != nil {
		t.Fatalf("encoder.Encode returned error: %v", err)
	}

	fp := &fakePipe{}

	tep := NewTcpEncodingPipe(fp, encoder)

	if passErr := tep.Pass(input); passErr != nil {
		t.Fatalf("Pass returned unexpected error: %v", passErr)
	}

	result := fp.buf.Bytes()
	if !bytes.Equal(result, expectedPacket.Payload) {
		t.Errorf("expected output %v, got %v", expectedPacket.Payload, result)
	}
}
