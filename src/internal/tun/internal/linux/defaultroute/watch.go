//go:build linux

package defaultroute

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Offsets of struct rtmsg (<linux/rtnetlink.h>) fields in a route message payload.
const (
	familyOffset                  = 0 // rtm_family
	destinationPrefixLengthOffset = 1 // rtm_dst_len
	routeTypeOffset               = 7 // rtm_type
)

type route struct {
	gateway netip.Addr // RTA_GATEWAY: address of the next hop router, invalid for on-link paths
	oif     uint32     // RTA_OIF: index of the output interface
}

func Watch(ctx context.Context, addr netip.Addr) (<-chan error, error) {
	addr = addr.Unmap()
	recvBuffer := make([]byte, unix.Getpagesize())
	request := buildRouteRequest(addr)
	dst := &unix.SockaddrNetlink{Family: unix.AF_NETLINK}
	initialRoute, err := lookupRoute(recvBuffer, request, dst)
	if err != nil {
		return nil, err
	}
	errCh := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cur, err := lookupRoute(recvBuffer, request, dst)
				if err != nil {
					if ctx.Err() == nil {
						errCh <- fmt.Errorf("no route to the server: %w", err)
					}
					return
				}
				if cur == initialRoute {
					continue
				}
				if ctx.Err() == nil {
					errCh <- nil
				}
				return
			}
		}
	}()
	return errCh, nil
}

func lookupRoute(recvBuffer, request []byte, dst unix.Sockaddr) (route, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return route{}, err
	}
	defer func() {
		_ = unix.Close(fd)
	}()
	if err := unix.Sendto(fd, request, 0, dst); err != nil {
		return route{}, err
	}
	n, _, err := unix.Recvfrom(fd, recvBuffer, 0)
	if err != nil {
		return route{}, err
	}
	messages, err := syscall.ParseNetlinkMessage(recvBuffer[:n])
	if err != nil {
		return route{}, err
	}
	return routeReply(messages)
}

func buildRouteRequest(addr netip.Addr) []byte {
	destination := addr.AsSlice()
	attrLength := unix.SizeofRtAttr + len(destination)
	length := unix.SizeofNlMsghdr + unix.SizeofRtMsg + attrLength
	request := make([]byte, 0, length)
	request = binary.NativeEndian.AppendUint32(request, uint32(length))
	request = binary.NativeEndian.AppendUint16(request, unix.RTM_GETROUTE)
	request = binary.NativeEndian.AppendUint16(request, unix.NLM_F_REQUEST)
	request = binary.NativeEndian.AppendUint32(request, 1) // sequence number
	request = binary.NativeEndian.AppendUint32(request, 0) // port ID, filled in by the kernel
	rtmsg := make([]byte, unix.SizeofRtMsg)
	rtmsg[familyOffset] = byte(addrFamily(addr))
	rtmsg[destinationPrefixLengthOffset] = byte(addr.BitLen())
	request = append(request, rtmsg...)
	request = binary.NativeEndian.AppendUint16(request, uint16(attrLength))
	request = binary.NativeEndian.AppendUint16(request, unix.RTA_DST)
	return append(request, destination...)
}

func addrFamily(addr netip.Addr) int {
	if addr.Unmap().Is4() {
		return unix.AF_INET
	}
	return unix.AF_INET6
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
	var r route
	for _, a := range attrs {
		switch a.Attr.Type {
		case unix.RTA_GATEWAY:
			r.gateway, _ = netip.AddrFromSlice(a.Value)
		case unix.RTA_OIF:
			r.oif = binary.NativeEndian.Uint32(a.Value)
		}
	}
	return r, nil
}
