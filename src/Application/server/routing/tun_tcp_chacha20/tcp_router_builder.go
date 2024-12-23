package tun_tcp_chacha20

import (
	"context"
	"sync"
	"tungo/Application/boundary"
	"tungo/Domain/settings"
)

type TCPRouterBuilder struct {
}

func NewTCPRouter() *TCPRouter {
	return &TCPRouter{}
}

func (r *TCPRouter) UseSettings(settings settings.ConnectionSettings) *TCPRouter {
	if r.err != nil {
		return r
	}

	r.settings = settings
	return r
}

func (r *TCPRouter) UseTun(tun boundary.TunAdapter) *TCPRouter {
	if r.err != nil {
		return r
	}

	r.tun = tun
	return r
}

func (r *TCPRouter) UseLocalIPMap(localIpMap *sync.Map) *TCPRouter {
	if r.err != nil {
		return r
	}

	r.localIpMap = localIpMap
	return r
}

func (r *TCPRouter) UseLocalIPToSessionMap(localIpToSessionMap *sync.Map) *TCPRouter {
	if r.err != nil {
		return r
	}

	r.localIpToSessionMap = localIpToSessionMap
	return r
}

func (r *TCPRouter) UseContext(ctx context.Context) *TCPRouter {
	if r.err != nil {
		return r
	}

	r.ctx = ctx
	return r
}

func (r *TCPRouter) Build() (*TCPRouter, error) {
	if r.err != nil {
		return nil, r.err
	}

	return r, nil
}
