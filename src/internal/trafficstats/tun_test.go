package trafficstats

import (
	"errors"
	"testing"
)

type meteredTunStub struct {
	readData []byte
	readErr  error
	writeN   int
	writeErr error
}

func (t *meteredTunStub) Read(p []byte) (int, error) {
	return copy(p, t.readData), t.readErr
}

func (t *meteredTunStub) Write([]byte) (int, error) {
	return t.writeN, t.writeErr
}

func TestWrapTunCountsActualIOBytes(t *testing.T) {
	readErr := errors.New("read")
	writeErr := errors.New("write")
	raw := &meteredTunStub{
		readData: []byte{1, 2, 3, 4},
		readErr:  readErr,
		writeN:   3,
		writeErr: writeErr,
	}
	collector := NewCollector(0, 0)
	previous := Global()
	SetGlobal(collector)
	defer SetGlobal(previous)

	tun := WrapTun(raw)
	if n, err := tun.Read(make([]byte, 8)); n != 4 || !errors.Is(err, readErr) {
		t.Fatalf("Read() = (%d, %v), want (4, %v)", n, err, readErr)
	}
	if n, err := tun.Write(make([]byte, 8)); n != 3 || !errors.Is(err, writeErr) {
		t.Fatalf("Write() = (%d, %v), want (3, %v)", n, err, writeErr)
	}
	snapshot := collector.Snapshot()
	if snapshot.TXBytesTotal != 4 || snapshot.RXBytesTotal != 3 {
		t.Fatalf("snapshot = %+v, want TX=4 RX=3", snapshot)
	}
}

func TestWrapTunWithoutCollectorReturnsOriginal(t *testing.T) {
	previous := Global()
	SetGlobal(nil)
	defer SetGlobal(previous)

	raw := &meteredTunStub{}
	if wrapped := WrapTun(raw); wrapped != raw {
		t.Fatal("WrapTun should not allocate a decorator without a collector")
	}
}
