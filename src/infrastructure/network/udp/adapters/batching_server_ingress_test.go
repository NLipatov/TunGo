package adapters

import (
	"bytes"
	"net/netip"
	"testing"
	appudp "tungo/application/network/udp"
)

type fakeBatchSource struct {
	payloads [][]byte
	addrs    []netip.AddrPort
	flags    []int
}

func (f *fakeBatchSource) ReadBatch(msgs []batchReadMessage) (int, error) {
	n := len(f.payloads)
	if n > len(msgs) {
		n = len(msgs)
	}
	for i := 0; i < n; i++ {
		copy(msgs[i].Buffer, f.payloads[i])
		msgs[i].N = len(f.payloads[i])
		msgs[i].Addr = f.addrs[i]
		msgs[i].Flags = f.flags[i]
	}
	return n, nil
}

func TestBatchingServerIngress_ReadBatch_FillsCallerBuffers(t *testing.T) {
	source := &fakeBatchSource{
		payloads: [][]byte{
			[]byte("alpha"),
			[]byte("beta"),
		},
		addrs: []netip.AddrPort{
			netip.MustParseAddrPort("10.0.0.1:1111"),
			netip.MustParseAddrPort("10.0.0.2:2222"),
		},
		flags: []int{1, 2},
	}

	ingress := &batchingServerIngress{source: source}
	buffers := [2][16]byte{}
	packets := []appudp.Packet{
		{Data: buffers[0][:]},
		{Data: buffers[1][:]},
	}

	n, err := ingress.ReadBatch(packets)
	if err != nil {
		t.Fatalf("ReadBatch failed: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 packets, got %d", n)
	}

	if !bytes.Equal(packets[0].Data, []byte("alpha")) {
		t.Fatalf("unexpected first packet: %q", packets[0].Data)
	}
	if packets[0].Addr != source.addrs[0] || packets[0].Flags != source.flags[0] {
		t.Fatalf("unexpected first metadata: addr=%v flags=%d", packets[0].Addr, packets[0].Flags)
	}

	if !bytes.Equal(packets[1].Data, []byte("beta")) {
		t.Fatalf("unexpected second packet: %q", packets[1].Data)
	}
	if packets[1].Addr != source.addrs[1] || packets[1].Flags != source.flags[1] {
		t.Fatalf("unexpected second metadata: addr=%v flags=%d", packets[1].Addr, packets[1].Flags)
	}
}
