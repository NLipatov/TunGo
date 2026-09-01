//go:build windows

package ipcfg

import (
	"net/netip"
	"reflect"
	"testing"
)

func TestV4ParseDNS(t *testing.T) {
	v := new(V4)
	tests := []struct {
		name    string
		servers []string
		want    []netip.Addr
	}{
		{
			name:    "servers",
			servers: []string{" 1.1.1.1 ", "8.8.8.8"},
			want:    []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("8.8.8.8")},
		},
		{name: "clear"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := v.parseDNS(test.servers)
			if err != nil {
				t.Fatalf("parseDNS() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("DNS addresses = %v, want %v", got, test.want)
			}
		})
	}
}

func TestV4ParseDNSRejectsInvalidAddresses(t *testing.T) {
	v := new(V4)
	for _, servers := range [][]string{
		{"not-an-ip"},
		{"2001:db8::1"},
		{"::ffff:192.0.2.1"},
		{" "},
	} {
		_, err := v.parseDNS(servers)
		if err == nil {
			t.Fatalf("parseDNS(%q) error = nil", servers)
		}
	}
}
