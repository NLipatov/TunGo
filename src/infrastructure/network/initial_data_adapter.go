package network

import (
	"tungo/application"
)

type InitialDataAdapter struct {
	adapter     application.ConnectionAdapter
	initialData []byte
}

func NewInitialDataAdapter(
	adapter application.ConnectionAdapter,
	initialData []byte,
) *InitialDataAdapter {
	return &InitialDataAdapter{
		adapter:     adapter,
		initialData: initialData,
	}
}

func (ua *InitialDataAdapter) Write(data []byte) (int, error) {
	return ua.adapter.Write(data)
}

func (ua *InitialDataAdapter) Read(buffer []byte) (int, error) {
	if len(ua.initialData) > 0 {
		n := copy(buffer, ua.initialData)
		ua.initialData = ua.initialData[n:]
		if len(ua.initialData) == 0 {
			ua.initialData = nil
		}
		return n, nil
	}
	return ua.adapter.Read(buffer)
}

func (ua *InitialDataAdapter) Close() error {
	return ua.adapter.Close()
}
