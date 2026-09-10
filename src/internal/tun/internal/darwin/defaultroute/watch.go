//go:build darwin

package defaultroute

import (
	"context"
	"errors"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

func Watch(ctx context.Context) (<-chan struct{}, error) {
	fd, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
	if err != nil {
		return nil, err
	}

	unix.CloseOnExec(fd)
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	changed := make(chan struct{})
	go func() {
		defer func() {
			_ = unix.Close(fd)
		}()

		poll := []unix.PollFd{{
			Fd:     int32(fd),
			Events: unix.POLLIN,
		}}
		buffer := make([]byte, 64*1024)

		for ctx.Err() == nil {
			n, err := unix.Poll(poll, 500)
			if errors.Is(err, unix.EINTR) || n == 0 {
				continue
			}
			if err != nil {
				return
			}

			n, err = unix.Read(fd, buffer)
			if errors.Is(err, unix.EAGAIN) {
				continue
			}
			if err != nil {
				return
			}

			messages, err := route.ParseRIB(route.RIBTypeRoute, buffer[:n])
			if err != nil {
				continue
			}
			for _, message := range messages {
				if isDefaultRouteChange(message) {
					close(changed)
					return
				}
			}
		}
	}()

	return changed, nil
}

func isDefaultRouteChange(message route.Message) bool {
	routeMessage, ok := message.(*route.RouteMessage)
	if !ok {
		return false
	}
	switch routeMessage.Type {
	case unix.RTM_ADD, unix.RTM_DELETE, unix.RTM_CHANGE:
	default:
		return false
	}
	if len(routeMessage.Addrs) <= unix.RTAX_NETMASK {
		return false
	}

	destination := routeMessage.Addrs[unix.RTAX_DST]
	mask := routeMessage.Addrs[unix.RTAX_NETMASK]
	switch destination := destination.(type) {
	case *route.Inet4Addr:
		return destination.IP == [4]byte{} && isZeroIPv4Mask(mask)
	case *route.Inet6Addr:
		return destination.IP == [16]byte{} && isZeroIPv6Mask(mask)
	default:
		return false
	}
}

func isZeroIPv4Mask(mask route.Addr) bool {
	if mask == nil {
		return true
	}
	value, ok := mask.(*route.Inet4Addr)
	return ok && value.IP == [4]byte{}
}

func isZeroIPv6Mask(mask route.Addr) bool {
	if mask == nil {
		return true
	}
	value, ok := mask.(*route.Inet6Addr)
	return ok && value.IP == [16]byte{}
}
