package pipes

import (
	"bytes"
	"errors"
	"testing"

	"tungo/crypto/chacha20"
)

type tcpDecodingPipeFakePipe struct {
	buf bytes.Buffer
}

func (fp *tcpDecodingPipeFakePipe) Pass(data []byte) error {
	_, err := fp.buf.Write(data)
	return err
}

type tcpDecodingPipeErrorTCPEncoder struct{}

func (e *tcpDecodingPipeErrorTCPEncoder) Encode(_ []byte) (*chacha20.TCPPacket, error) {
	return nil, errors.New("encode error")
}

func (e *tcpDecodingPipeErrorTCPEncoder) Decode(_ []byte) (*chacha20.TCPPacket, error) {
	return nil, errors.New("decode error")
}

func TestTCPDecodingPipe_Success(t *testing.T) {
	payload := []byte("hello")
	encoder := &chacha20.DefaultTCPEncoder{}
	encodedPacket, err := encoder.Encode(payload)
	if err != nil {
		t.Fatalf("encoder.Encode returned error: %v", err)
	}

	fp := &tcpDecodingPipeFakePipe{}
	decodingPipe := NewTCPDecodingPipe(fp, encoder)

	if passErr := decodingPipe.Pass(encodedPacket.Payload); passErr != nil {
		t.Fatalf("Pass returned error: %v", passErr)
	}

	result := fp.buf.Bytes()
	resultWithoutLengthPrefix := result[4:]
	if !bytes.Equal(resultWithoutLengthPrefix, payload) {
		t.Errorf("expected %q, got %q", payload, result)
	}
}

func TestTCPDecodingPipe_Error(t *testing.T) {
	errEncoder := &tcpDecodingPipeErrorTCPEncoder{}

	fp := &tcpDecodingPipeFakePipe{}
	decodingPipe := NewTCPDecodingPipe(fp, errEncoder)

	if err := decodingPipe.Pass([]byte("dummy")); err == nil {
		t.Fatal("expected error, got nil")
	} else if err.Error() != "decode error" {
		t.Errorf("expected error 'decode error', got %q", err.Error())
	}
}
