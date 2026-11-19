//go:build linux

package defaultroute

import (
	"context"
	"encoding/binary"
	"errors"
	"net/netip"
	"slices"
	"syscall"

	"golang.org/x/sys/unix"
)

// Offsets of struct rtmsg (<linux/rtnetlink.h>) fields in a route message payload.
const (
	destinationPrefixLengthOffset = 1 // rtm_dst_len
	routingTableOffset            = 4 // rtm_table
	routeTypeOffset               = 7 // rtm_type
)

type route struct {
	destination netip.Addr
	bits        uint8
	gateway     netip.Addr
	oif         uint32
}

func Watch(ctx context.Context, serverAddr netip.Addr) (<-chan error, error) {
	fd, err := unix.Socket(
		unix.AF_NETLINK,
		unix.SOCK_RAW|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK,
		unix.NETLINK_ROUTE,
	)
	if err != nil {
		return nil, err
	}

	if err := unix.Bind(fd, &unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
		Groups: routeGroup(serverAddr),
	}); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	serverAddr = serverAddr.Unmap()
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

			n, _, err = unix.Recvfrom(fd, buffer, 0)
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
				continue
			}
			// The kernel drops notifications when the socket buffer overflows,
			// for example while split routes are added. Lost notifications may
			// hide a default route change, so the routing table is checked instead.
			if errors.Is(err, unix.ENOBUFS) {
				if ignore(serverAddr) {
					continue
				}
				errCh <- nil
				return
			}
			if err != nil {
				if ctx.Err() == nil {
					errCh <- err
				}
				return
			}

			messages, err := syscall.ParseNetlinkMessage(buffer[:n])
			if err != nil {
				if ctx.Err() == nil {
					errCh <- err
				}
				return
			}
			if slices.ContainsFunc(messages, isDefaultRouteChange) {
				if ignore(serverAddr) {
					continue
				}
				errCh <- nil
				return
			}
		}
	}()

	return errCh, nil
}

func ignore(serverAddr netip.Addr) bool {
	routes, err := mainRoutes(addrFamily(serverAddr))
	return err == nil && keepsServerPath(routes, serverAddr)
}

// keepsServerPath reports whether a default route still leads through the
// gateway and interface of the host route to the server, which carries the tunnel.
func keepsServerPath(routes []route, serverAddr netip.Addr) bool {
	i := slices.IndexFunc(routes, func(r route) bool {
		return int(r.bits) == serverAddr.BitLen() && r.destination == serverAddr
	})
	// A zero interface index means the route was not parsed (multipath or nexthop object),
	// so its path is unknown and cannot prove the path is kept.
	if i < 0 || routes[i].oif == 0 {
		return false
	}
	server := routes[i]
	return slices.ContainsFunc(routes, func(r route) bool {
		// A server route without a gateway is on-link: a default route through
		// the same interface shows that interface still serves the network.
		return r.bits == 0 && r.oif == server.oif &&
			(!server.gateway.IsValid() || r.gateway == server.gateway)
	})
}

func routeGroup(addr netip.Addr) uint32 {
	if addr.Unmap().Is4() {
		return unix.RTMGRP_IPV4_ROUTE
	}
	return unix.RTMGRP_IPV6_ROUTE
}

func addrFamily(addr netip.Addr) int {
	if addr.Unmap().Is4() {
		return unix.AF_INET
	}
	return unix.AF_INET6
}

func isDefaultRouteChange(message syscall.NetlinkMessage) bool {
	return (message.Header.Type == unix.RTM_NEWROUTE ||
		message.Header.Type == unix.RTM_DELROUTE) &&
		len(message.Data) >= unix.SizeofRtMsg &&
		message.Data[destinationPrefixLengthOffset] == 0 &&
		message.Data[routingTableOffset] == unix.RT_TABLE_MAIN
}

func mainRoutes(family int) ([]route, error) {
	rib, err := syscall.NetlinkRIB(unix.RTM_GETROUTE, family)
	if err != nil {
		return nil, err
	}
	messages, err := syscall.ParseNetlinkMessage(rib)
	if err != nil {
		return nil, err
	}
	return routesFrom(messages)
}

func routesFrom(messages []syscall.NetlinkMessage) ([]route, error) {
	routes := make([]route, 0, len(messages))
	for _, m := range messages {
		r, ok, err := routeFrom(m)
		if err != nil {
			return nil, err
		}
		if ok {
			routes = append(routes, r)
		}
	}
	return routes, nil
}

func routeFrom(m syscall.NetlinkMessage) (route, bool, error) {
	if m.Header.Type != unix.RTM_NEWROUTE ||
		len(m.Data) < unix.SizeofRtMsg ||
		m.Data[routingTableOffset] != unix.RT_TABLE_MAIN ||
		m.Data[routeTypeOffset] != unix.RTN_UNICAST {
		return route{}, false, nil
	}
	attrs, err := syscall.ParseNetlinkRouteAttr(&m)
	if err != nil {
		return route{}, false, err
	}
	r := route{bits: m.Data[destinationPrefixLengthOffset]}
	for _, a := range attrs {
		switch a.Attr.Type {
		case unix.RTA_DST:
			r.destination, _ = netip.AddrFromSlice(a.Value)
		case unix.RTA_GATEWAY:
			r.gateway, _ = netip.AddrFromSlice(a.Value)
		case unix.RTA_OIF:
			r.oif = binary.NativeEndian.Uint32(a.Value)
		}
	}
	return r, true, nil
}
