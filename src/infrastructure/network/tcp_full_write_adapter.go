package network

import (
	"io"
	"tungo/application"
)

type TcpFullWriteAdapter struct {
	adapter application.ConnectionAdapter
}

func NewTcpFullWriteAdapter(
	adapter application.ConnectionAdapter,
) application.ConnectionAdapter {
	return &TcpFullWriteAdapter{
		adapter: adapter,
	}
}

func (ta *TcpFullWriteAdapter) Write(data []byte) (int, error) {
	off := 0
	for off < len(data) {
		n, err := ta.adapter.Write(data[off:])
		if n > 0 {
			off += n
		}
		if err != nil {
			return off, err
		}
		if n == 0 {
			return off, io.ErrShortWrite
		}
	}
	return off, nil
}

func (ta *TcpFullWriteAdapter) Read(buffer []byte) (int, error) {
	return ta.adapter.Read(buffer)
}

func (ta *TcpFullWriteAdapter) Close() error {
	return ta.adapter.Close()
}
