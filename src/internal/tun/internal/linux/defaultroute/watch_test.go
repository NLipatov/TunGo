//go:build linux

package defaultroute

import (
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSamePath(t *testing.T) {
	gateway := netip.MustParseAddr("10.0.0.1")
	otherGateway := netip.MustParseAddr("10.0.0.2")
	path := route{gateway: gateway, oif: 2}

	tests := []struct {
		name    string
		path    route
		current route
		want    bool
	}{
		{name: "same gateway and interface", path: path, current: route{gateway: gateway, oif: 2}, want: true},
		{name: "another gateway", path: path, current: route{gateway: otherGateway, oif: 2}},
		{name: "another interface", path: path, current: route{gateway: gateway, oif: 3}},
		{name: "on-link path through the same interface", path: route{oif: 2}, current: route{oif: 2}, want: true},
		{name: "on-link path now through a gateway", path: route{oif: 2}, current: route{gateway: gateway, oif: 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := samePath(tt.path, tt.current); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDrain(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM|unix.SOCK_NONBLOCK, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() {
		_ = unix.Close(fds[0])
		_ = unix.Close(fds[1])
	})
	for range 3 {
		if _, err := unix.Write(fds[1], []byte("notification")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if err := drain(fds[0], make([]byte, 64)); err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	if _, _, err := unix.Recvfrom(fds[0], make([]byte, 64), 0); !errors.Is(err, unix.EAGAIN) {
		t.Fatalf("got %v, want %v", err, unix.EAGAIN)
	}
}

func TestDrainClosedSocket(t *testing.T) {
	if err := drain(-1, make([]byte, 64)); !errors.Is(err, unix.EBADF) {
		t.Fatalf("got %v, want %v", err, unix.EBADF)
	}
}

func TestRouteRequest(t *testing.T) {
	for _, server := range []netip.Addr{netip.MustParseAddr("198.51.100.7"), netip.MustParseAddr("2001:db8::7")} {
		t.Run(server.String(), func(t *testing.T) {
			messages, err := syscall.ParseNetlinkMessage(routeRequest(server))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(messages) != 1 {
				t.Fatalf("got %d messages, want 1", len(messages))
			}
			m := messages[0]
			if m.Header.Type != unix.RTM_GETROUTE || m.Header.Flags != unix.NLM_F_REQUEST {
				t.Fatalf("got type %d flags %d, want a route lookup request", m.Header.Type, m.Header.Flags)
			}
			if int(m.Data[familyOffset]) != addrFamily(server) || int(m.Data[destinationPrefixLengthOffset]) != server.BitLen() {
				t.Fatalf("got family %d prefix length %d", m.Data[familyOffset], m.Data[destinationPrefixLengthOffset])
			}
			// The standard parser reads attributes of route notifications only.
			m.Header.Type = unix.RTM_NEWROUTE
			r, err := routeAttrs(m)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r.destination != server {
				t.Fatalf("got destination %v, want %v", r.destination, server)
			}
		})
	}
}

func TestRouteReply(t *testing.T) {
	server := netip.MustParseAddr("198.51.100.7")
	gateway := netip.MustParseAddr("10.0.0.1")
	errno := -int32(unix.ENETUNREACH)
	errorReply := syscall.NetlinkMessage{
		Header: syscall.NlMsghdr{Type: unix.NLMSG_ERROR},
		Data:   binary.NativeEndian.AppendUint32(nil, uint32(errno)),
	}

	t.Run("route", func(t *testing.T) {
		got, err := routeReply([]syscall.NetlinkMessage{
			routeMessage(unix.RTM_NEWROUTE, 32, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, dstAttr(server), gatewayAttr(gateway), oifAttr(2)),
		})
		want := route{destination: server, bits: 32, gateway: gateway, oif: 2}
		if err != nil || got != want {
			t.Fatalf("got %+v, %v, want %+v", got, err, want)
		}
	})
	t.Run("kernel error", func(t *testing.T) {
		if _, err := routeReply([]syscall.NetlinkMessage{errorReply}); !errors.Is(err, unix.ENETUNREACH) {
			t.Fatalf("got %v, want %v", err, unix.ENETUNREACH)
		}
	})
	t.Run("short error", func(t *testing.T) {
		short := syscall.NetlinkMessage{Header: syscall.NlMsghdr{Type: unix.NLMSG_ERROR}}
		if _, err := routeReply([]syscall.NetlinkMessage{short}); err == nil {
			t.Fatal("got nil error, want error")
		}
	})
	t.Run("route from a policy routing table", func(t *testing.T) {
		got, err := routeReply([]syscall.NetlinkMessage{
			routeMessage(unix.RTM_NEWROUTE, 32, 100, unix.RTN_UNICAST, dstAttr(server), gatewayAttr(gateway), oifAttr(2)),
		})
		want := route{destination: server, bits: 32, gateway: gateway, oif: 2}
		if err != nil || got != want {
			t.Fatalf("got %+v, %v, want %+v", got, err, want)
		}
	})
	t.Run("local route", func(t *testing.T) {
		got, err := routeReply([]syscall.NetlinkMessage{
			routeMessage(unix.RTM_NEWROUTE, 32, unix.RT_TABLE_LOCAL, unix.RTN_LOCAL, dstAttr(server), oifAttr(1)),
		})
		want := route{destination: server, bits: 32, oif: 1}
		if err != nil || got != want {
			t.Fatalf("got %+v, %v, want %+v", got, err, want)
		}
	})
	t.Run("route without interface", func(t *testing.T) {
		reply := routeMessage(unix.RTM_NEWROUTE, 32, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, dstAttr(server), gatewayAttr(gateway))
		if _, err := routeReply([]syscall.NetlinkMessage{reply}); err == nil {
			t.Fatal("got nil error, want error")
		}
	})
	t.Run("unexpected message", func(t *testing.T) {
		reply := syscall.NetlinkMessage{Header: syscall.NlMsghdr{Type: unix.RTM_NEWLINK}}
		if _, err := routeReply([]syscall.NetlinkMessage{reply}); err == nil {
			t.Fatal("got nil error, want error")
		}
	})
	t.Run("unreachable route", func(t *testing.T) {
		reply := routeMessage(unix.RTM_NEWROUTE, 32, unix.RT_TABLE_MAIN, unix.RTN_UNREACHABLE, dstAttr(server))
		if _, err := routeReply([]syscall.NetlinkMessage{reply}); err == nil {
			t.Fatal("got nil error, want error")
		}
	})
	t.Run("empty", func(t *testing.T) {
		if _, err := routeReply(nil); err == nil {
			t.Fatal("got nil error, want error")
		}
	})
}

func TestGroups(t *testing.T) {
	tests := []struct {
		addr netip.Addr
		want uint32
	}{
		{addr: netip.MustParseAddr("192.0.2.1"), want: unix.RTMGRP_LINK | unix.RTMGRP_IPV4_IFADDR | unix.RTMGRP_IPV4_ROUTE},
		{addr: netip.MustParseAddr("::ffff:192.0.2.1"), want: unix.RTMGRP_LINK | unix.RTMGRP_IPV4_IFADDR | unix.RTMGRP_IPV4_ROUTE},
		{addr: netip.MustParseAddr("2001:db8::1"), want: unix.RTMGRP_LINK | unix.RTMGRP_IPV6_IFADDR | unix.RTMGRP_IPV6_ROUTE},
	}

	for _, tt := range tests {
		t.Run(tt.addr.String(), func(t *testing.T) {
			if got := groups(tt.addr); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServerPathAsksTheKernel(t *testing.T) {
	_, err := serverPath(netip.MustParseAddr("192.0.2.1"))
	// A host without a route to the address gets the kernel's error instead.
	if err != nil && !errors.Is(err, unix.ENETUNREACH) {
		t.Fatalf("got %v, want a route or %v", err, unix.ENETUNREACH)
	}
}

func TestServerPathToLoopback(t *testing.T) {
	lo, err := net.InterfaceByName("lo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := serverPath(netip.MustParseAddr("127.0.0.1"))
	if err != nil || got.oif != uint32(lo.Index) || got.gateway.IsValid() {
		t.Fatalf("got %+v, %v, want a route through interface %d", got, err, lo.Index)
	}
}

func TestWatchStarts(t *testing.T) {
	// Through a local proxy the client sees the proxy's loopback address as the server.
	errCh, err := Watch(t.Context(), netip.MustParseAddr("127.0.0.1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if errCh == nil {
		t.Fatal("got nil channel")
	}
}

// routingTableOffset is the offset of rtm_table in struct rtmsg.
const routingTableOffset = 4

type routeAttr struct {
	kind  uint16
	value []byte
}

func dstAttr(addr netip.Addr) routeAttr {
	return routeAttr{kind: unix.RTA_DST, value: addr.AsSlice()}
}

func gatewayAttr(addr netip.Addr) routeAttr {
	return routeAttr{kind: unix.RTA_GATEWAY, value: addr.AsSlice()}
}

func oifAttr(index uint32) routeAttr {
	return routeAttr{kind: unix.RTA_OIF, value: binary.NativeEndian.AppendUint32(nil, index)}
}

func routeMessage(kind uint16, dstLen, table, routeType byte, attrs ...routeAttr) syscall.NetlinkMessage {
	data := make([]byte, unix.SizeofRtMsg)
	data[destinationPrefixLengthOffset] = dstLen
	data[routingTableOffset] = table
	data[routeTypeOffset] = routeType
	for _, attr := range attrs {
		length := unix.SizeofRtAttr + len(attr.value)
		data = binary.NativeEndian.AppendUint16(data, uint16(length))
		data = binary.NativeEndian.AppendUint16(data, attr.kind)
		data = append(data, attr.value...)
		aligned := (length + unix.RTA_ALIGNTO - 1) &^ (unix.RTA_ALIGNTO - 1)
		data = append(data, make([]byte, aligned-length)...)
	}
	return syscall.NetlinkMessage{Header: syscall.NlMsghdr{Type: kind}, Data: data}
}
