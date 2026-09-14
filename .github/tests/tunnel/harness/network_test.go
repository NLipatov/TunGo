package main

import (
	"net/netip"
	"reflect"
	"testing"
)

func TestPreservedPrefixesExcludeOnlyTestNetworks(t *testing.T) {
	excluded := netip.MustParsePrefix("198.18.0.0/15")
	prefixes := preservedPrefixes(excluded)
	addresses := uint64(1) << (32 - excluded.Bits())
	for i, prefix := range prefixes {
		if prefix.Bits() < 2 {
			t.Errorf("fixture would claim a TunGo split default: %s", prefix)
		}
		if prefix.Overlaps(excluded) {
			t.Errorf("fixture would bypass TunGo for test traffic: %s", prefix)
		}
		for _, other := range prefixes[:i] {
			if prefix.Overlaps(other) {
				t.Errorf("overlapping preserved routes: %s and %s", prefix, other)
			}
		}
		addresses += uint64(1) << (32 - prefix.Bits())
	}
	if addresses != uint64(1)<<32 {
		t.Fatalf("preserved routes leave holes outside test networks: %d addresses covered", addresses)
	}
}

func TestDarwinSnapshotKeepsRoutesAndIgnoresNeighborChurn(t *testing.T) {
	before := `Routing tables
Internet:
Destination Gateway Flags Netif Expire
default 192.168.1.1 UGScg en0
192.168.1.1 aa:bb:cc:dd:ee:ff UHLWIir en0 1199
198.18.0.2 198.19.0.1 UHW3I utun4 20
0/1 198.19.0.1 UGSc utun4
128.0/1 198.19.0.1 UGSc utun4
`
	after := `Destination Gateway Flags Netif Expire
128.0/1 198.19.0.1 UGSc utun4
default 192.168.1.1 UGScg en0
0/1 198.19.0.1 UGSc utun4
192.168.1.20 11:22:33:44:55:66 UHLWI en0 30
`
	want := []string{"0/1 198.19.0.1 UGSc utun4", "128.0/1 198.19.0.1 UGSc utun4", "default 192.168.1.1 UGScg en0"}
	for _, output := range []string{before, after} {
		if got := darwinRoutes(output); !reflect.DeepEqual(got, want) {
			t.Fatalf("snapshot %v != %v", got, want)
		}
	}
}
