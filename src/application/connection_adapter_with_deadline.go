package application

import "time"

type ConnectionAdapterWithDeadline interface {
	ConnectionAdapter
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}
