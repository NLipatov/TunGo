package adapter

import (
	"errors"
	"time"
)

type DeadlinedAdapter struct {
	adapter                 *Adapter
	readWindow, writeWindow time.Duration
}

func NewDeadlinedAdapter(
	adapter *Adapter,
	readWindow, writeWindow time.Duration,
) (*DeadlinedAdapter, error) {
	if readWindow < 0 {
		return nil, errors.New("readWindow cannot be < 0")
	}
	if writeWindow < 0 {
		return nil, errors.New("writeWindow cannot be < 0")
	}
	return &DeadlinedAdapter{
		adapter:     adapter,
		readWindow:  readWindow,
		writeWindow: writeWindow,
	}, nil
}

func (a *DeadlinedAdapter) Write(data []byte) (int, error) {
	if a.writeWindow > 0 {
		if err := a.adapter.SetWriteDeadline(time.Now().Add(a.writeWindow)); err != nil {
			return 0, err
		}
	} else {
		if err := a.adapter.SetWriteDeadline(time.Time{}); err != nil {
			return 0, err
		}
	}
	return a.adapter.Write(data)
}

func (a *DeadlinedAdapter) Read(buffer []byte) (int, error) {
	if a.readWindow > 0 {
		if err := a.adapter.SetReadDeadline(time.Now().Add(a.readWindow)); err != nil {
			return 0, err
		}
	} else {
		if err := a.adapter.SetReadDeadline(time.Time{}); err != nil {
			return 0, err
		}
	}
	return a.adapter.Read(buffer)
}

func (a *DeadlinedAdapter) Close() error {
	return a.adapter.Close()
}
