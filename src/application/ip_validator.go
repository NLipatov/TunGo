package application

import (
	"net"
	"tungo/infrastructure/network/ip"
)

type IPValidator interface {
	ValidateIP(ver ip.Version, ip net.IP) error
}
