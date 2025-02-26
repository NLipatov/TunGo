package pipes

import "tungo/crypto/chacha20"

type TCPDecodingPipe struct {
	encoder chacha20.TCPEncoder
	pipe    Pipe
}

func NewTCPDecodingPipe(pipe Pipe, encoder chacha20.TCPEncoder) Pipe {
	return &TCPDecodingPipe{
		encoder: encoder,
		pipe:    pipe,
	}
}

func (tdp *TCPDecodingPipe) Pass(data []byte) error {
	decoded, err := tdp.encoder.Decode(data)
	if err != nil {
		return err
	}

	return tdp.pipe.Pass(decoded.Payload)
}
