package tcp

import (
	"net"
	"time"
)

// readDeadlineConn refreshes the connection's read deadline before each Read.
type readDeadlineConn struct {
	net.Conn
	timeout time.Duration
}

// WithReadDeadline sets a new read deadline of now + timeout before every Read.
func WithReadDeadline(conn net.Conn, timeout time.Duration) net.Conn {
	return &readDeadlineConn{
		Conn:    conn,
		timeout: timeout,
	}
}

func (r *readDeadlineConn) Read(p []byte) (int, error) {
	_ = r.SetReadDeadline(time.Now().Add(r.timeout))
	return r.Conn.Read(p)
}
