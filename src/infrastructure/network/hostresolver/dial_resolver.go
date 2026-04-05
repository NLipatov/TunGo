package hostresolver

import (
	"fmt"
	"net"
)

type dialFunc func(network, address string) (net.Conn, error)

type DialResolver struct {
	dial dialFunc
}

func NewDialResolver() *DialResolver {
	return &DialResolver{dial: net.Dial}
}

func (r *DialResolver) ResolveIPv4() (string, error) {
	return r.resolve("udp4", "8.8.8.8:80")
}

func (r *DialResolver) ResolveIPv6() (string, error) {
	return r.resolve("udp6", "[2001:4860:4860::8888]:80")
}

func (r *DialResolver) resolve(network, endpoint string) (string, error) {
	conn, err := r.dial(network, endpoint)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = conn.Close()
	}()

	localAddr := conn.LocalAddr()
	udpAddr, ok := localAddr.(*net.UDPAddr)
	if !ok {
		return "", fmt.Errorf("unexpected local address type %T", localAddr)
	}
	return udpAddr.IP.String(), nil
}
