package server

import (
	"net/http"
	"tungo/infrastructure/network/ws"
	"tungo/infrastructure/settings"

	"github.com/coder/websocket"
)

// compile-time check
var _ ws.Upgrader = (*DefaultUpgrader)(nil)

type DefaultUpgrader struct{}

func NewDefaultUpgrader() *DefaultUpgrader { return &DefaultUpgrader{} }

func (a *DefaultUpgrader) Upgrade(w http.ResponseWriter, r *http.Request) (ws.Conn, error) {
	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return nil, err
	}
	wsConn.SetReadLimit(int64(settings.MTU + settings.TCPChacha20Overhead))
	return wsConn, nil
}
