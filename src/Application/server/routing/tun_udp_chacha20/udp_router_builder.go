package tun_udp_chacha20

import (
	"context"
	"fmt"
	"sync"
	"tungo/Application/boundary"
	"tungo/Domain/settings"
)

type UDPRouterBuilder struct {
}

func NewUDPRouter() *UDPRouter {
	return &UDPRouter{}
}

func (r *UDPRouter) UseSettings(settings settings.ConnectionSettings) *UDPRouter {
	if r.err != nil {
		return r
	}

	r.settings = settings
	return r
}

func (r *UDPRouter) UseTun(tun boundary.TunAdapter) *UDPRouter {
	if r.err != nil {
		return r
	}

	r.tun = tun
	return r
}

func (r *UDPRouter) UseClientAddrToInternalIP(clientAddrToInternalIP *sync.Map) *UDPRouter {
	if r.err != nil {
		return r
	}

	r.clientAddrToInternalIP = clientAddrToInternalIP
	return r
}

func (r *UDPRouter) UseLocalIPToSessionMap(intIPToSession *sync.Map) *UDPRouter {
	if r.err != nil {
		return r
	}

	r.intIPToSession = intIPToSession
	return r
}

func (r *UDPRouter) UseIntIPToUDPClientAddr(intIPToUDPClientAddr *sync.Map) *UDPRouter {
	if r.err != nil {
		return r
	}

	r.intIPToUDPClientAddr = intIPToUDPClientAddr
	return r
}

func (r *UDPRouter) UseContext(ctx context.Context) *UDPRouter {
	if r.err != nil {
		return r
	}

	r.ctx = ctx
	return r
}

func (r *UDPRouter) Build() (*UDPRouter, error) {
	if r.err != nil {
		return nil, r.err
	}

	if r.tun == nil {
		return nil, fmt.Errorf("tun must be set")
	}

	if r.intIPToSession == nil {
		r.intIPToSession = &sync.Map{}
	}

	if r.intIPToUDPClientAddr == nil {
		r.intIPToUDPClientAddr = &sync.Map{}
	}

	if r.clientAddrToInternalIP == nil {
		r.clientAddrToInternalIP = &sync.Map{}
	}

	if r.ctx == nil {
		return nil, fmt.Errorf("ctx must be set")
	}

	return r, nil
}
