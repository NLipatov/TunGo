package contracts

import (
	"net/http"
)

// Upgrader — upgrades HTTP to WebSocket and returns Conn.
type Upgrader interface {
	Upgrade(w http.ResponseWriter, r *http.Request) (Conn, error)
}
