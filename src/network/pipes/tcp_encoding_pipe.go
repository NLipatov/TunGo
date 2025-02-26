package pipes

import "tungo/crypto/chacha20"

type TcpEncodingPipe struct {
	encoder chacha20.TCPEncoder
	pipe    Pipe
}

func NewTcpEncodingPipe(pipe Pipe, encoder chacha20.TCPEncoder) Pipe {
	return &TcpEncodingPipe{
		encoder: encoder,
		pipe:    pipe,
	}
}

func (tep *TcpEncodingPipe) Pass(data []byte) error {
	tcpPacket, tcpPacketErr := tep.encoder.Encode(data)
	if tcpPacketErr != nil {
		return tcpPacketErr
	}

	return tep.pipe.Pass(tcpPacket.Payload)
}
