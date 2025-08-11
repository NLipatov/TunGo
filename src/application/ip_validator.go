package application

import (
	"net"
	"tungo/domain/network/ip"
)

type IPValidator interface {
	ValidateIP(ver ip.Version, ip net.IP) error
}
