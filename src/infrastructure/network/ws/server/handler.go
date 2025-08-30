package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"tungo/application"
	"tungo/infrastructure/network/ws"
	"tungo/infrastructure/network/ws/adapter"
)

// Handler — HTTP-handlers of incoming connections.
type Handler interface {
	Handle(w http.ResponseWriter, r *http.Request)
}

// Ensure Handler implements contracts.Handler.
var _ Handler = (*DefaultHandler)(nil)

// DefaultHandler upgrades HTTP connections to WebSocket and enqueues them as net.Conn adapters.
type DefaultHandler struct {
	upgrader ws.Upgrader
	queue    chan net.Conn
	logger   application.Logger
}

func NewDefaultHandler(
	upgrader ws.Upgrader,
	queue chan net.Conn,
	logger application.Logger,
) *DefaultHandler {
	return &DefaultHandler{
		upgrader: upgrader,
		queue:    queue,
		logger:   logger,
	}
}

func (h *DefaultHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// Try parse remote address.
	rAddr, err := h.addrFromRequest(r)
	if err != nil {
		if h.logger != nil {
			h.logger.Printf("bad remote addr: %v", err)
		}
		http.Error(w, "bad remote addr", http.StatusBadRequest)
		return
	}

	// Upgrade HTTP to WebSocket.
	wsConn, uErr := h.upgrader.Upgrade(w, r)
	if uErr != nil {
		if h.logger != nil {
			h.logger.Printf("upgrade failed: %v", uErr)
		}
		return
	}

	// Determine the listener's local address from the request context.
	// Fallback to an empty TCPAddr if absent (should not happen with net/http Server).
	local := net.Addr(&net.TCPAddr{})
	if la, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && la != nil {
		local = la
	}

	// Enqueue the adapted net.Conn, or reject on overflow.
	select {
	case h.queue <- adapter.NewDefaultAdapter(context.Background(), wsConn, local, rAddr):
	default:
		_ = wsConn.Close(CloseCodeQueueFull, "could not accept new connection")
	}
}

func (h *DefaultHandler) addrFromRequest(r *http.Request) (*net.TCPAddr, error) {
	host, port, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil, err
	}
	p, pErr := strconv.Atoi(port)
	if pErr != nil {
		return nil, errors.New("invalid remote port number")
	}
	return &net.TCPAddr{IP: net.ParseIP(host), Port: p}, nil
}
