package adapters

import (
	"time"

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

func (ua *InitialDataAdapter) SetDeadline(t time.Time) error {
	if d, ok := ua.adapter.(interface{ SetDeadline(time.Time) error }); ok {
		return d.SetDeadline(t)
	}
	return nil
}

func (ua *InitialDataAdapter) SetReadDeadline(t time.Time) error {
	if d, ok := ua.adapter.(interface{ SetReadDeadline(time.Time) error }); ok {
		return d.SetReadDeadline(t)
	}
	return nil
}

func (ua *InitialDataAdapter) SetWriteDeadline(t time.Time) error {
	if d, ok := ua.adapter.(interface{ SetWriteDeadline(time.Time) error }); ok {
		return d.SetWriteDeadline(t)
	}
	return nil
}
