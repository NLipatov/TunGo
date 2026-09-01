//go:build windows

package ipcfg

import (
	"net/netip"
	"reflect"
	"testing"
)

func TestV6ParseDNS(t *testing.T) {
	v := new(V6)
	tests := []struct {
		name    string
		servers []string
		want    []netip.Addr
	}{
		{
			name:    "servers",
			servers: []string{" 2606:4700:4700::1111 ", "2001:4860:4860::8888"},
			want: []netip.Addr{
				netip.MustParseAddr("2606:4700:4700::1111"),
				netip.MustParseAddr("2001:4860:4860::8888"),
			},
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

func TestV6ParseDNSRejectsInvalidAddresses(t *testing.T) {
	v := new(V6)
	for _, servers := range [][]string{
		{"not-an-ip"},
		{"192.0.2.1"},
		{"::ffff:192.0.2.1"},
		{" "},
	} {
		_, err := v.parseDNS(servers)
		if err == nil {
			t.Fatalf("parseDNS(%q) error = nil", servers)
		}
	}
}
