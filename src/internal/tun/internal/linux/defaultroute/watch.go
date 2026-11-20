//go:build linux

package defaultroute

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"syscall"

	"golang.org/x/sys/unix"
)

// Offsets of struct rtmsg (<linux/rtnetlink.h>) fields in a route message payload.
const (
	familyOffset                  = 0 // rtm_family
	destinationPrefixLengthOffset = 1 // rtm_dst_len
	routeTypeOffset               = 7 // rtm_type
)

type route struct {
	destination netip.Addr
	bits        uint8
	gateway     netip.Addr
	oif         uint32
}

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

	// Asked after subscribing, so a change in between is not missed.
	path, err := serverPath(serverAddr)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

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
			if err == nil {
				err = drain(fd, buffer)
			}
			if err != nil {
				if ctx.Err() == nil {
					errCh <- err
				}
				return
			}

			if keeps(serverAddr, path) {
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

// drain reads pending notifications, so a burst of them costs one route lookup.
// Their content is not needed: the kernel is asked for the path instead.
func drain(fd int, buffer []byte) error {
	for {
		_, _, err := unix.Recvfrom(fd, buffer, 0)
		switch {
		case errors.Is(err, unix.EAGAIN):
			return nil
		// The kernel drops notifications when the socket buffer overflows, for
		// example while split routes are added. The path is asked all the same.
		case err == nil || errors.Is(err, unix.EINTR) || errors.Is(err, unix.ENOBUFS):
			continue
		default:
			return err
		}
	}
}

// keeps reports whether the kernel still takes the path to the server.
func keeps(serverAddr netip.Addr, path route) bool {
	current, err := serverPath(serverAddr)
	return err == nil && samePath(path, current)
}

func samePath(path, current route) bool {
	return current.oif == path.oif && current.gateway == path.gateway
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
	if message.Header.Type != unix.RTM_NEWROUTE || len(message.Data) < unix.SizeofRtMsg {
		return route{}, fmt.Errorf("unexpected route reply of type %d", message.Header.Type)
	}
	// A local route leads to an address of this host, such as a loopback proxy.
	if routeType := message.Data[routeTypeOffset]; routeType != unix.RTN_UNICAST && routeType != unix.RTN_LOCAL {
		return route{}, fmt.Errorf("route reply of route type %d leads to no host", routeType)
	}
	r, err := routeAttrs(message)
	if err != nil {
		return route{}, err
	}
	if r.oif == 0 {
		return route{}, errors.New("route reply has no interface")
	}
	return r, nil
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
