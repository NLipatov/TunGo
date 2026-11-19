//go:build darwin

package defaultroute

import (
	"context"
	"errors"
	"net/netip"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

func Watch(ctx context.Context, serverAddr netip.Addr) (<-chan error, error) {
	fd, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
	if err != nil {
		return nil, err
	}

	unix.CloseOnExec(fd)
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	family := addrFamily(serverAddr)

	errCh := make(chan error, 1)
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
				if ctx.Err() == nil {
					errCh <- err
				}
				return
			}

			n, err = unix.Read(fd, buffer)
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
				continue
			}
			if err != nil {
				if ctx.Err() == nil {
					errCh <- err
				}
				return
			}

			messages, err := route.ParseRIB(route.RIBTypeRoute, buffer[:n])
			if err != nil {
				if ctx.Err() == nil {
					errCh <- err
				}
				return
			}
			for _, message := range messages {
				if changesDefaultRoute(message, family) {
					errCh <- nil
					return
				}
			}
		}
	}()

	return errCh, nil
}

func addrFamily(addr netip.Addr) int {
	if addr.Unmap().Is4() {
		return unix.AF_INET
	}
	return unix.AF_INET6
}

func changesDefaultRoute(message route.Message, family int) bool {
	routeMessage, ok := message.(*route.RouteMessage)
	if !ok || routeMessage.Err != nil {
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
	// Scoped default routes only serve sockets bound to their interface.
	// The connection to the server is unbound and follows the unscoped one.
	if routeMessage.Flags&unix.RTF_IFSCOPE != 0 {
		return false
	}

	destination := routeMessage.Addrs[unix.RTAX_DST]
	mask := routeMessage.Addrs[unix.RTAX_NETMASK]
	switch destination := destination.(type) {
	case *route.Inet4Addr:
		if family != unix.AF_INET {
			return false
		}
		return destination.IP == [4]byte{} && isZeroIPv4Mask(mask)
	case *route.Inet6Addr:
		if family != unix.AF_INET6 {
			return false
		}
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
