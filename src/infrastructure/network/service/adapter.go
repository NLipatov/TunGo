package service

import (
	"tungo/application"
)

// Adapter is used to detect and handle service frames
type Adapter struct {
	adapter application.ConnectionAdapter
	handler Handler
}

func NewDefaultAdapter(
	adapter application.ConnectionAdapter,
	handler Handler,
) *Adapter {
	return &Adapter{
		adapter: adapter, // TUN device adapter
		handler: handler,
	}
}

func (a *Adapter) Write(data []byte) (int, error) {
	return a.adapter.Write(
		a.handler.Handle(data), //handler will either rewrite data or not
	)
}

func (a *Adapter) Read(buffer []byte) (int, error) {
	return a.adapter.Read(buffer)
}

func (a *Adapter) Close() error {
	return a.adapter.Close()
}
