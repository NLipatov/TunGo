//go:build linux

package defaultroute

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"slices"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestClassify(t *testing.T) {
	server := netip.MustParseAddr("198.51.100.7")
	gateway := netip.MustParseAddr("10.0.0.1")
	malformed := routeMessage(unix.RTM_DELROUTE, 32, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, dstAttr(server))
	// Declare an attribute longer than the remaining payload.
	binary.NativeEndian.PutUint16(malformed.Data[unix.SizeofRtMsg:], 64)

	tests := []struct {
		name    string
		message syscall.NetlinkMessage
		want    verdict
	}{
		{name: "deleted link", message: headerMessage(unix.RTM_DELLINK), want: recheck},
		{name: "deleted address", message: headerMessage(unix.RTM_DELADDR), want: recheck},
		{name: "new link", message: headerMessage(unix.RTM_NEWLINK), want: unrelated},
		{name: "new address", message: headerMessage(unix.RTM_NEWADDR), want: unrelated},
		{name: "added default route", message: routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST), want: recheck},
		{name: "deleted default route", message: routeMessage(unix.RTM_DELROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST), want: recheck},
		{name: "default route in another table", message: routeMessage(unix.RTM_NEWROUTE, 0, 100, unix.RTN_UNICAST), want: unrelated},
		{name: "split route", message: routeMessage(unix.RTM_NEWROUTE, 1, unix.RT_TABLE_MAIN, unix.RTN_UNICAST), want: unrelated},
		{name: "short route message", message: syscall.NetlinkMessage{Header: syscall.NlMsghdr{Type: unix.RTM_NEWROUTE}, Data: make([]byte, unix.SizeofRtMsg-1)}, want: unrelated},
		{
			name:    "deleted server route",
			message: routeMessage(unix.RTM_DELROUTE, 32, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, dstAttr(server), gatewayAttr(gateway), oifAttr(2)),
			want:    changed,
		},
		{
			name:    "added server route",
			message: routeMessage(unix.RTM_NEWROUTE, 32, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, dstAttr(server), gatewayAttr(gateway), oifAttr(2)),
			want:    unrelated,
		},
		{
			name:    "deleted route to another host",
			message: routeMessage(unix.RTM_DELROUTE, 32, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, dstAttr(gateway), oifAttr(2)),
			want:    unrelated,
		},
		{
			name:    "deleted server route in another table",
			message: routeMessage(unix.RTM_DELROUTE, 32, 100, unix.RTN_UNICAST, dstAttr(server)),
			want:    unrelated,
		},
		{name: "malformed deleted host route", message: malformed, want: recheck},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classify(tt.message, server); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKeepsPath(t *testing.T) {
	gateway := netip.MustParseAddr("10.0.0.1")
	otherGateway := netip.MustParseAddr("10.0.0.2")
	path := route{gateway: gateway, oif: 2}

	tests := []struct {
		name   string
		path   route
		routes []route
		want   bool
	}{
		{
			name:   "default route through the path",
			path:   path,
			routes: []route{{gateway: gateway, oif: 2}},
			want:   true,
		},
		{
			name:   "better default route next to the path",
			path:   path,
			routes: []route{{gateway: otherGateway, oif: 3}, {gateway: gateway, oif: 2}},
			want:   true,
		},
		{
			name:   "default route through another gateway",
			path:   path,
			routes: []route{{gateway: otherGateway, oif: 2}},
		},
		{
			name:   "default route through another interface",
			path:   path,
			routes: []route{{gateway: gateway, oif: 3}},
		},
		{
			name:   "host route through the path is not a default route",
			path:   path,
			routes: []route{{destination: otherGateway, bits: 32, gateway: gateway, oif: 2}},
		},
		{
			name: "no default route",
			path: path,
		},
		{
			name:   "unknown path",
			routes: []route{{}},
		},
		{
			name:   "on-link path and default route through its interface",
			path:   route{oif: 2},
			routes: []route{{gateway: gateway, oif: 2}},
			want:   true,
		},
		{
			name:   "on-link path and default route through another interface",
			path:   route{oif: 2},
			routes: []route{{gateway: gateway, oif: 3}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keepsPath(tt.routes, tt.path); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
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
	t.Run("route from another table", func(t *testing.T) {
		reply := routeMessage(unix.RTM_NEWROUTE, 32, 100, unix.RTN_UNICAST, dstAttr(server), oifAttr(2))
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

func TestRoutesFrom(t *testing.T) {
	gateway := netip.MustParseAddr("10.0.0.3")
	server := netip.MustParseAddr("198.51.100.7")

	messages := []syscall.NetlinkMessage{
		routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(gateway), oifAttr(2)),
		routeMessage(unix.RTM_NEWROUTE, 32, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, dstAttr(server), gatewayAttr(gateway), oifAttr(2)),
		routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, oifAttr(7)),
		routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(netip.MustParseAddr("fe80::1")), oifAttr(3)),
		// Skipped: other table, non-unicast and deleted routes.
		routeMessage(unix.RTM_NEWROUTE, 0, 100, unix.RTN_UNICAST, gatewayAttr(gateway), oifAttr(2)),
		routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNREACHABLE),
		routeMessage(unix.RTM_DELROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(gateway), oifAttr(2)),
	}
	want := []route{
		{gateway: gateway, oif: 2},
		{destination: server, bits: 32, gateway: gateway, oif: 2},
		{oif: 7},
		{gateway: netip.MustParseAddr("fe80::1"), oif: 3},
	}

	got, err := routesFrom(messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestRoutesFromMalformedAttribute(t *testing.T) {
	message := routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, oifAttr(2))
	// Declare an attribute longer than the remaining payload.
	binary.NativeEndian.PutUint16(message.Data[unix.SizeofRtMsg:], 64)

	if _, err := routesFrom([]syscall.NetlinkMessage{message}); err == nil {
		t.Fatal("got nil error, want error")
	}
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
	r, err := serverPath(netip.MustParseAddr("192.0.2.1"))
	// A host without a route to the address gets the kernel's error instead.
	if err == nil && r.oif == 0 {
		t.Fatalf("got %+v, want a route with an interface", r)
	}
}

func TestWatchStarts(t *testing.T) {
	errCh, err := Watch(t.Context(), netip.MustParseAddr("192.0.2.1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if errCh == nil {
		t.Fatal("got nil channel")
	}
}

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

func headerMessage(kind uint16) syscall.NetlinkMessage {
	return syscall.NetlinkMessage{Header: syscall.NlMsghdr{Type: kind}}
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
