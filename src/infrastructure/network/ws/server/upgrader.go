package server

import (
	"net/http"
	"tungo/infrastructure/network/ws/contracts"
	"tungo/infrastructure/settings"

	"github.com/coder/websocket"
)

type DefaultUpgrader struct{}

func NewDefaultUpgrader() *DefaultUpgrader { return &DefaultUpgrader{} }

func (a *DefaultUpgrader) Upgrade(w http.ResponseWriter, r *http.Request) (contracts.Conn, error) {
	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return nil, err
	}
	wsConn.SetReadLimit(int64(settings.DefaultEthernetMTU + settings.TCPChacha20Overhead))
	return wsConn, nil
}
