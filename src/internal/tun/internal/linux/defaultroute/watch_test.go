//go:build linux

package defaultroute

import (
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestIsDefaultRouteChange(t *testing.T) {
	tests := []struct {
		name    string
		kind    uint16
		dstLen  byte
		tableID byte
		want    bool
	}{
		{name: "default route", kind: unix.RTM_NEWROUTE, tableID: unix.RT_TABLE_MAIN, want: true},
		{name: "deleted default route", kind: unix.RTM_DELROUTE, tableID: unix.RT_TABLE_MAIN, want: true},
		{name: "custom table default route", kind: unix.RTM_NEWROUTE, tableID: 100, want: false},
		{name: "split route", kind: unix.RTM_NEWROUTE, dstLen: 1, tableID: unix.RT_TABLE_MAIN, want: false},
		{name: "address event", kind: unix.RTM_NEWADDR, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, unix.SizeofRtMsg)
			data[1] = tt.dstLen
			data[4] = tt.tableID
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
