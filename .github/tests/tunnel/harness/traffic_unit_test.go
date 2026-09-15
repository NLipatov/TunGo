package main

import (
	"context"
	"fmt"
	"net"
	"testing"
)

func TestTraceHop(t *testing.T) {
	for _, test := range []struct {
		output, address string
		hop             int
		want            bool
	}{
		{"traceroute to 198.19.0.1\n 1  * * *", "198.19.0.1", 1, false},
		{" 1  198.19.0.10  0.123 ms", "198.19.0.1", 1, false},
		{" 1  198.19.0.1  0.123 ms\n 2  198.18.0.2  0.234 ms", "198.19.0.1", 1, true},
		{" 1    <1 ms    <1 ms    <1 ms  fd73:7467:6f:1::1", "fd73:7467:6f:1::1", 1, true},
		{" 2  198.19.0.1  0.234 ms", "198.19.0.1", 1, false},
	} {
		if got := traceHop(test.output, test.hop, test.address); got != test.want {
			t.Errorf("traceHop(%q, %d, %q) = %t, want %t", test.output, test.hop, test.address, got, test.want)
		}
	}
}

func TestCheckClientCleanupRejectsExistingInterface(t *testing.T) {
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback == 0 {
			continue
		}
		// A known interface must be detected independently of its IP addresses.
		want := fmt.Sprintf("client TUN interface %s (index %d) remains", iface.Name, iface.Index)
		if err := checkClientCleanup(context.Background(), iface, nil); err == nil || err.Error() != want {
			t.Fatalf("expected %q, got %v", want, err)
		}
		return
	}
	t.Fatal("no loopback interface found")
}
