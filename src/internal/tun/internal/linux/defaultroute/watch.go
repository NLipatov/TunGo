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
	gateway     netip.Addr
	oif, metric uint32
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

	family := addrFamily(serverAddr)
	// Without a snapshot every default route event counts as a change.
	prev, prevOk, _ := defaultRoute(family)

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
			if slices.ContainsFunc(messages, changesDefaultRoute) {
				if ignore(prev, prevOk, family) {
					continue
				}
				errCh <- nil
				return
			}
		}
	}()

	return errCh, nil
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

func ignore(prev route, prevOk bool, family int) bool {
	if !prevOk {
		return false
	}
	cur, curOk, err := defaultRoute(family)
	return err == nil && curOk &&
		cur.gateway == prev.gateway && cur.oif == prev.oif &&
		// A zero interface index means the route was not parsed (multipath or nexthop object),
		// so its identity is unknown and cannot prove the route is unchanged.
		cur.oif != 0
}

func changesDefaultRoute(message syscall.NetlinkMessage) bool {
	return (message.Header.Type == unix.RTM_NEWROUTE ||
		message.Header.Type == unix.RTM_DELROUTE) &&
		len(message.Data) >= unix.SizeofRtMsg &&
		message.Data[destinationPrefixLengthOffset] == 0 &&
		message.Data[routingTableOffset] == unix.RT_TABLE_MAIN
}

func defaultRoute(family int) (route, bool, error) {
	rib, err := syscall.NetlinkRIB(unix.RTM_GETROUTE, family)
	if err != nil {
		return route{}, false, err
	}
	messages, err := syscall.ParseNetlinkMessage(rib)
	if err != nil {
		return route{}, false, err
	}
	return bestDefaultRoute(messages)
}

func bestDefaultRoute(messages []syscall.NetlinkMessage) (route, bool, error) {
	best, found := route{}, false
	for _, message := range messages {
		r, ok, err := routeFrom(message)
		if err != nil {
			return route{}, false, err
		}
		if !ok {
			continue
		}
		if !found || best.metric > r.metric {
			found = true
			best = r
		}
	}
	return best, found, nil
}

func routeFrom(m syscall.NetlinkMessage) (route, bool, error) {
	if m.Header.Type != unix.RTM_NEWROUTE ||
		len(m.Data) < unix.SizeofRtMsg ||
		m.Data[destinationPrefixLengthOffset] != 0 ||
		m.Data[routingTableOffset] != unix.RT_TABLE_MAIN ||
		m.Data[routeTypeOffset] != unix.RTN_UNICAST {
		return route{}, false, nil
	}
	attrs, err := syscall.ParseNetlinkRouteAttr(&m)
	if err != nil {
		return route{}, false, err
	}
	r := route{}
	for _, a := range attrs {
		switch a.Attr.Type {
		case unix.RTA_GATEWAY:
			r.gateway, _ = netip.AddrFromSlice(a.Value)
		case unix.RTA_OIF:
			r.oif = binary.NativeEndian.Uint32(a.Value)
		case unix.RTA_PRIORITY:
			r.metric = binary.NativeEndian.Uint32(a.Value)
		}
	}
	return r, true, nil
}
