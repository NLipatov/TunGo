//go:build linux

package defaultroute

import (
	"cmp"
	"encoding/binary"
	"net/netip"
	"slices"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestIsDefaultRouteChange(t *testing.T) {
	tests := []struct {
		name      string
		kind      uint16
		family    byte
		dstLen    byte
		tableID   byte
		shortData bool
		want      bool
	}{
		{name: "added IPv4 default route", kind: unix.RTM_NEWROUTE, family: unix.AF_INET, tableID: unix.RT_TABLE_MAIN, want: true},
		{name: "deleted IPv6 default route", kind: unix.RTM_DELROUTE, family: unix.AF_INET6, tableID: unix.RT_TABLE_MAIN, want: true},
		{name: "custom table default route", kind: unix.RTM_NEWROUTE, family: unix.AF_INET, tableID: 100},
		{name: "split route", kind: unix.RTM_NEWROUTE, family: unix.AF_INET, dstLen: 1, tableID: unix.RT_TABLE_MAIN},
		{name: "address event", kind: unix.RTM_NEWADDR, family: unix.AF_INET, tableID: unix.RT_TABLE_MAIN},
		{name: "short route message", kind: unix.RTM_NEWROUTE, shortData: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataLen := unix.SizeofRtMsg
			if tt.shortData {
				dataLen--
			}
			data := make([]byte, dataLen)
			if !tt.shortData {
				data[0] = tt.family
				data[1] = tt.dstLen
				data[4] = tt.tableID
			}
			message := syscall.NetlinkMessage{
				Header: syscall.NlMsghdr{Type: tt.kind},
				Data:   data,
			}
			if got := isDefaultRouteChange(message); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
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

func TestKeepsServerPath(t *testing.T) {
	server := netip.MustParseAddr("198.51.100.7")
	gateway := netip.MustParseAddr("10.0.0.1")
	otherGateway := netip.MustParseAddr("10.0.0.2")
	serverRoute := route{destination: server, bits: 32, gateway: gateway, oif: 2}

	tests := []struct {
		name   string
		server netip.Addr
		routes []route
		want   bool
	}{
		{
			name:   "default route through the server route gateway and interface",
			routes: []route{serverRoute, {gateway: gateway, oif: 2}},
			want:   true,
		},
		{
			name:   "better default route next to the server route path",
			routes: []route{serverRoute, {gateway: otherGateway, oif: 3}, {gateway: gateway, oif: 2}},
			want:   true,
		},
		{
			name:   "default route through another gateway",
			routes: []route{serverRoute, {gateway: otherGateway, oif: 2}},
		},
		{
			name:   "default route through another interface",
			routes: []route{serverRoute, {gateway: gateway, oif: 3}},
		},
		{
			name:   "no default route",
			routes: []route{serverRoute},
		},
		{
			name:   "no server route",
			routes: []route{{gateway: gateway, oif: 2}},
		},
		{
			name:   "host route to another address",
			routes: []route{{destination: otherGateway, bits: 32, gateway: gateway, oif: 2}, {gateway: gateway, oif: 2}},
		},
		{
			name:   "server route with unknown interface",
			routes: []route{{destination: server, bits: 32}, {}},
		},
		{
			name:   "on-link server route and default route through its interface",
			routes: []route{{destination: server, bits: 32, oif: 2}, {gateway: gateway, oif: 2}},
			want:   true,
		},
		{
			name:   "on-link server route and default route through another interface",
			routes: []route{{destination: server, bits: 32, oif: 2}, {gateway: gateway, oif: 3}},
		},
		{
			name:   "IPv6 server route",
			server: netip.MustParseAddr("2001:db8::7"),
			routes: []route{
				{destination: netip.MustParseAddr("2001:db8::7"), bits: 128, gateway: netip.MustParseAddr("fe80::1"), oif: 2},
				{gateway: netip.MustParseAddr("fe80::1"), oif: 2},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keepsServerPath(tt.routes, cmp.Or(tt.server, server)); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
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

func TestRouteGroup(t *testing.T) {
	tests := []struct {
		addr netip.Addr
		want uint32
	}{
		{addr: netip.MustParseAddr("192.0.2.1"), want: unix.RTMGRP_IPV4_ROUTE},
		{addr: netip.MustParseAddr("::ffff:192.0.2.1"), want: unix.RTMGRP_IPV4_ROUTE},
		{addr: netip.MustParseAddr("2001:db8::1"), want: unix.RTMGRP_IPV6_ROUTE},
	}

	for _, tt := range tests {
		t.Run(tt.addr.String(), func(t *testing.T) {
			if got := routeGroup(tt.addr); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
