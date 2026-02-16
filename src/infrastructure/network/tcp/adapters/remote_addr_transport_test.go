package adapters

import (
	"net/netip"
	"testing"

	framelimit "tungo/domain/network/ip/frame_limit"
)

type plainTransport struct {
	LengthPrefixFramingAdapterMockConn
}

func TestRemoteAddrTransport_RemoteAddrPort(t *testing.T) {
	addr := netip.MustParseAddrPort("198.51.100.1:4321")
	inner := &plainTransport{}
	rat := NewRemoteAddrTransport(inner, addr)

	if got := rat.RemoteAddrPort(); got != addr {
		t.Fatalf("RemoteAddrPort() = %v, want %v", got, addr)
	}
	if got := rat.Unwrap(); got != inner {
		t.Fatalf("Unwrap() = %T, want %T", got, inner)
	}
}

func TestRemoteAddrTransport_DelegatesReadWriteClose(t *testing.T) {
	inner := &plainTransport{}
	inner.readData = []byte{0x00, 0x01, 0x42}
	rat := NewRemoteAddrTransport(inner, netip.AddrPort{})

	buf := make([]byte, 10)
	n, err := rat.Read(buf)
	if err != nil || n != 3 {
		t.Fatalf("Read() = %d, %v; want 3, nil", n, err)
	}

	if _, err := rat.Write([]byte{0xAA}); err != nil {
		t.Fatalf("Write() err = %v", err)
	}

	if err := rat.Close(); err != nil {
		t.Fatalf("Close() err = %v", err)
	}
}

func TestLengthPrefixFramingAdapter_RemoteAddrPort_Delegates(t *testing.T) {
	addr := netip.MustParseAddrPort("203.0.113.5:9999")
	inner := &plainTransport{}
	rat := NewRemoteAddrTransport(inner, addr)
	fa, err := NewLengthPrefixFramingAdapter(rat, framelimit.Cap(1500))
	if err != nil {
		t.Fatal(err)
	}

	if got := fa.RemoteAddrPort(); got != addr {
		t.Fatalf("RemoteAddrPort() = %v, want %v", got, addr)
	}
}

func TestLengthPrefixFramingAdapter_RemoteAddrPort_NoInner(t *testing.T) {
	inner := &plainTransport{}
	fa, err := NewLengthPrefixFramingAdapter(inner, framelimit.Cap(1500))
	if err != nil {
		t.Fatal(err)
	}

	if got := fa.RemoteAddrPort(); got != (netip.AddrPort{}) {
		t.Fatalf("RemoteAddrPort() = %v, want zero", got)
	}
}
