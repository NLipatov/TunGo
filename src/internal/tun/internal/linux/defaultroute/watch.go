//go:build linux

package defaultroute

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"syscall"

	"golang.org/x/sys/unix"
)

// Offsets of struct rtmsg (<linux/rtnetlink.h>) fields in a route message payload.
const (
	familyOffset                  = 0 // rtm_family
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

// verdict is what a notification means for the path to the server, ordered by severity.
type verdict int

const (
	unrelated verdict = iota // the notification cannot affect the path
	recheck                  // the path may be gone
	changed                  // the path is gone
)

func Watch(ctx context.Context, serverAddr netip.Addr) (<-chan error, error) {
	serverAddr = serverAddr.Unmap()
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
		Groups: groups(serverAddr),
	}); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	family := addrFamily(serverAddr)
	// Asked after subscribing, so a change in between is not missed. Without
	// the path every relevant notification counts as a change.
	path, _ := serverPath(serverAddr)

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

			v := unrelated
			n, _, err = unix.Recvfrom(fd, buffer, 0)
			switch {
			case errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR):
				continue
			// The kernel drops notifications when the socket buffer overflows,
			// for example while split routes are added. Lost notifications may
			// hide a change, so the routing table is checked instead.
			case errors.Is(err, unix.ENOBUFS):
				v = recheck
			case err != nil:
				if ctx.Err() == nil {
					errCh <- err
				}
				return
			default:
				messages, err := syscall.ParseNetlinkMessage(buffer[:n])
				if err != nil {
					if ctx.Err() == nil {
						errCh <- err
					}
					return
				}
				for _, message := range messages {
					v = max(v, classify(message, serverAddr))
				}
			}

			if v == unrelated || (v == recheck && keeps(path, family)) {
				continue
			}
			if ctx.Err() == nil {
				errCh <- nil
			}
			return
		}
	}()

	return errCh, nil
}

func groups(addr netip.Addr) uint32 {
	if addr.Unmap().Is4() {
		return unix.RTMGRP_LINK | unix.RTMGRP_IPV4_IFADDR | unix.RTMGRP_IPV4_ROUTE
	}
	return unix.RTMGRP_LINK | unix.RTMGRP_IPV6_IFADDR | unix.RTMGRP_IPV6_ROUTE
}

func addrFamily(addr netip.Addr) int {
	if addr.Unmap().Is4() {
		return unix.AF_INET
	}
	return unix.AF_INET6
}

func classify(message syscall.NetlinkMessage, serverAddr netip.Addr) verdict {
	switch message.Header.Type {
	case unix.RTM_DELLINK, unix.RTM_DELADDR:
		// The kernel removes routes of a deleted link or address without notifications.
		return recheck
	case unix.RTM_NEWROUTE, unix.RTM_DELROUTE:
		return classifyRoute(message, serverAddr)
	default:
		return unrelated
	}
}

func classifyRoute(message syscall.NetlinkMessage, serverAddr netip.Addr) verdict {
	if len(message.Data) < unix.SizeofRtMsg ||
		message.Data[routingTableOffset] != unix.RT_TABLE_MAIN {
		return unrelated
	}
	switch int(message.Data[destinationPrefixLengthOffset]) {
	case 0:
		return recheck
	case serverAddr.BitLen():
		if message.Header.Type != unix.RTM_DELROUTE {
			return unrelated
		}
		r, err := routeAttrs(message)
		if err != nil {
			return recheck
		}
		// The host route to the server is pinned on connect; without it the
		// tunnel traffic to the server would follow the split routes.
		if r.destination == serverAddr {
			return changed
		}
	}
	return unrelated
}

func keeps(path route, family int) bool {
	routes, err := mainRoutes(family)
	return err == nil && keepsPath(routes, path)
}

// keepsPath reports whether a default route still leads through the gateway
// and interface of the path to the server.
func keepsPath(routes []route, path route) bool {
	// A zero interface index means the kernel did not resolve the path.
	if path.oif == 0 {
		return false
	}
	// Multipath defaults carry their next hops in RTA_MULTIPATH and parse with
	// a zero interface index, so they never match and cannot prove the path is kept.
	return slices.ContainsFunc(routes, func(r route) bool {
		// A path without a gateway is on-link: a default route through the
		// same interface shows that interface still serves the network.
		return r.bits == 0 && r.oif == path.oif &&
			(!path.gateway.IsValid() || r.gateway == path.gateway)
	})
}

// serverPath asks the kernel which route it takes to the server, as `ip route get` does.
func serverPath(serverAddr netip.Addr) (route, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return route{}, err
	}
	defer func() {
		_ = unix.Close(fd)
	}()

	if err := unix.Sendto(fd, routeRequest(serverAddr), 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return route{}, err
	}
	buffer := make([]byte, unix.Getpagesize())
	n, _, err := unix.Recvfrom(fd, buffer, 0)
	if err != nil {
		return route{}, err
	}
	messages, err := syscall.ParseNetlinkMessage(buffer[:n])
	if err != nil {
		return route{}, err
	}
	return routeReply(messages)
}

func routeRequest(serverAddr netip.Addr) []byte {
	destination := serverAddr.AsSlice()
	attrLength := unix.SizeofRtAttr + len(destination)
	length := unix.SizeofNlMsghdr + unix.SizeofRtMsg + attrLength
	request := make([]byte, 0, length)
	request = binary.NativeEndian.AppendUint32(request, uint32(length))
	request = binary.NativeEndian.AppendUint16(request, unix.RTM_GETROUTE)
	request = binary.NativeEndian.AppendUint16(request, unix.NLM_F_REQUEST)
	request = binary.NativeEndian.AppendUint32(request, 1) // sequence number
	request = binary.NativeEndian.AppendUint32(request, 0) // port ID, filled in by the kernel
	rtmsg := make([]byte, unix.SizeofRtMsg)
	rtmsg[familyOffset] = byte(addrFamily(serverAddr))
	rtmsg[destinationPrefixLengthOffset] = byte(serverAddr.BitLen())
	request = append(request, rtmsg...)
	request = binary.NativeEndian.AppendUint16(request, uint16(attrLength))
	request = binary.NativeEndian.AppendUint16(request, unix.RTA_DST)
	return append(request, destination...)
}

func routeReply(messages []syscall.NetlinkMessage) (route, error) {
	if len(messages) == 0 {
		return route{}, errors.New("empty route reply")
	}
	message := messages[0]
	if message.Header.Type == unix.NLMSG_ERROR {
		// The payload starts with the negative errno as int32.
		if len(message.Data) < 4 {
			return route{}, errors.New("short route error reply")
		}
		return route{}, syscall.Errno(-int32(binary.NativeEndian.Uint32(message.Data)))
	}
	r, ok, err := routeFrom(message)
	if err != nil {
		return route{}, err
	}
	if !ok {
		return route{}, fmt.Errorf("route reply of type %d is not a main table unicast route", message.Header.Type)
	}
	return r, nil
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
	r, err := routeAttrs(m)
	return r, err == nil, err
}

func routeAttrs(m syscall.NetlinkMessage) (route, error) {
	attrs, err := syscall.ParseNetlinkRouteAttr(&m)
	if err != nil {
		return route{}, err
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
	return r, nil
}
