//go:build linux

package defaultroute

import (
	"net/netip"
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
				t.Fatalf("isDefaultRouteChange() = %v, want %v", got, tt.want)
			}
		})
	}
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
				t.Fatalf("routeGroup() = %v, want %v", got, tt.want)
			}
		})
	}
}
