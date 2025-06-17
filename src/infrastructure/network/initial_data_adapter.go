package network

import "tungo/application"

// InitialDataAdapter is a decorator on application.ConnectionAdapter type, which has some initial data that will be returned on first Read operation
type InitialDataAdapter struct {
	adapter     application.ConnectionAdapter
	initialData []byte
}

func NewInitialDataAdapter(adapter application.ConnectionAdapter, initialData []byte) *InitialDataAdapter {
	return &InitialDataAdapter{
		adapter: adapter, initialData: initialData,
	}
}

func (ua *InitialDataAdapter) Write(data []byte) (int, error) {
	return ua.adapter.Write(data)
}

func (ua *InitialDataAdapter) Read(buffer []byte) (int, error) {
	if ua.initialData != nil && len(ua.initialData) > 0 {
		n := copy(buffer, ua.initialData)
		if n < len(ua.initialData) {
			// if it's a partial read(initial data still has something to read)
			ua.initialData = ua.initialData[n:]
		} else {
			// allow GC to collect ua.initialData
			ua.initialData = nil
		}
		return n, nil
	}
	return ua.adapter.Read(buffer)
}

func (ua *InitialDataAdapter) Close() error {
	return ua.adapter.Close()
}
