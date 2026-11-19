//go:build linux

package defaultroute

import (
	"encoding/binary"
	"net/netip"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestChangesDefaultRoute(t *testing.T) {
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
			if got := changesDefaultRoute(message); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBestDefaultRoute(t *testing.T) {
	gateway := netip.MustParseAddr("10.0.0.3")
	otherGateway := netip.MustParseAddr("192.168.1.1")

	tests := []struct {
		name      string
		messages  []syscall.NetlinkMessage
		want      route
		wantFound bool
	}{
		{
			name:      "no default route",
			messages:  []syscall.NetlinkMessage{routeMessage(unix.RTM_NEWROUTE, 24, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(gateway), oifAttr(2))},
			wantFound: false,
		},
		{
			name: "single default route",
			messages: []syscall.NetlinkMessage{
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(gateway), oifAttr(2), metricAttr(101)),
			},
			want:      route{gateway: gateway, oif: 2, metric: 101},
			wantFound: true,
		},
		{
			name: "lowest metric wins when it comes last",
			messages: []syscall.NetlinkMessage{
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(otherGateway), oifAttr(3), metricAttr(600)),
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(gateway), oifAttr(2), metricAttr(100)),
			},
			want:      route{gateway: gateway, oif: 2, metric: 100},
			wantFound: true,
		},
		{
			name: "same route with penalized metric",
			messages: []syscall.NetlinkMessage{
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(gateway), oifAttr(2), metricAttr(20101)),
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(gateway), oifAttr(2), metricAttr(101)),
			},
			want:      route{gateway: gateway, oif: 2, metric: 101},
			wantFound: true,
		},
		{
			name: "non-default routes are skipped",
			messages: []syscall.NetlinkMessage{
				routeMessage(unix.RTM_NEWROUTE, 1, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(otherGateway), oifAttr(3)),
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(gateway), oifAttr(2), metricAttr(101)),
			},
			want:      route{gateway: gateway, oif: 2, metric: 101},
			wantFound: true,
		},
		{
			name: "other table, non-unicast and deleted routes are skipped",
			messages: []syscall.NetlinkMessage{
				routeMessage(unix.RTM_NEWROUTE, 0, 100, unix.RTN_UNICAST, gatewayAttr(otherGateway), oifAttr(3)),
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNREACHABLE, metricAttr(1)),
				routeMessage(unix.RTM_DELROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(otherGateway), oifAttr(3)),
			},
			wantFound: false,
		},
		{
			name: "default route without gateway",
			messages: []syscall.NetlinkMessage{
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, oifAttr(7)),
			},
			want:      route{oif: 7},
			wantFound: true,
		},
		{
			name: "IPv6 gateway",
			messages: []syscall.NetlinkMessage{
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(netip.MustParseAddr("fe80::1")), oifAttr(2)),
			},
			want:      route{gateway: netip.MustParseAddr("fe80::1"), oif: 2},
			wantFound: true,
		},
		{
			name: "equal metrics with different paths are ambiguous",
			messages: []syscall.NetlinkMessage{
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(gateway), oifAttr(2), metricAttr(1024)),
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(otherGateway), oifAttr(2), metricAttr(1024)),
			},
			wantFound: false,
		},
		{
			name: "equal metrics with the same path are not ambiguous",
			messages: []syscall.NetlinkMessage{
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(gateway), oifAttr(2), metricAttr(1024)),
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(gateway), oifAttr(2), metricAttr(1024)),
			},
			want:      route{gateway: gateway, oif: 2, metric: 1024},
			wantFound: true,
		},
		{
			name: "lower metric resolves earlier ambiguity",
			messages: []syscall.NetlinkMessage{
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(otherGateway), oifAttr(3), metricAttr(600)),
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(netip.MustParseAddr("172.16.0.1")), oifAttr(4), metricAttr(600)),
				routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, gatewayAttr(gateway), oifAttr(2), metricAttr(100)),
			},
			want:      route{gateway: gateway, oif: 2, metric: 100},
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found, err := bestDefaultRoute(tt.messages)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if found != tt.wantFound || got != tt.want {
				t.Fatalf("got %+v, %v, want %+v, %v", got, found, tt.want, tt.wantFound)
			}
		})
	}
}

func TestBestDefaultRouteMalformedAttribute(t *testing.T) {
	message := routeMessage(unix.RTM_NEWROUTE, 0, unix.RT_TABLE_MAIN, unix.RTN_UNICAST, oifAttr(2))
	// Declare an attribute longer than the remaining payload.
	binary.NativeEndian.PutUint16(message.Data[unix.SizeofRtMsg:], 64)

	if _, _, err := bestDefaultRoute([]syscall.NetlinkMessage{message}); err == nil {
		t.Fatal("got nil error, want error")
	}
}

type routeAttr struct {
	kind  uint16
	value []byte
}

func gatewayAttr(addr netip.Addr) routeAttr {
	return routeAttr{kind: unix.RTA_GATEWAY, value: addr.AsSlice()}
}

func oifAttr(index uint32) routeAttr {
	return routeAttr{kind: unix.RTA_OIF, value: binary.NativeEndian.AppendUint32(nil, index)}
}

func metricAttr(metric uint32) routeAttr {
	return routeAttr{kind: unix.RTA_PRIORITY, value: binary.NativeEndian.AppendUint32(nil, metric)}
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

func TestWatchStarts(t *testing.T) {
	errCh, err := Watch(t.Context(), netip.MustParseAddr("192.0.2.1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if errCh == nil {
		t.Fatal("got nil channel")
	}
}

func TestIgnoreUnchangedRoutingTable(t *testing.T) {
	snapshot, ok, err := defaultRoute(unix.AF_INET)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || snapshot.oif == 0 {
		t.Skip("no IPv4 default route with a known interface")
	}
	if !ignore(snapshot, ok, unix.AF_INET) {
		t.Fatal("got change for an unchanged routing table, want ignore")
	}
}
