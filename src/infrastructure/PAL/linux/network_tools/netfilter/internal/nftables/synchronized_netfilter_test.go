package nftables_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tungo/application"
	"tungo/infrastructure/PAL/linux/network_tools/netfilter/internal/nftables" // adjust if your path differs
)

// stubNF is a stand-in Netfilter implementation used by tests.
// It records call counts, injects optional errors, and can simulate
// delays to amplify race conditions.
type stubNF struct {
	// active tracks the number of concurrent entries into the implementation.
	active int32
	// seenConcurrent is set to 1 if there was a concurrent entry observed.
	seenConcurrent int32

	// per-method call counters
	cMasqOn, cMasqOff               int32
	cFwdTunToDevOn, cFwdTunToDevOff int32
	cFwdDevToTunOn, cFwdDevToTunOff int32
	cClamp                          int32

	// per-method errors to return when set
	errMasqOn, errMasqOff               error
	errFwdTunToDevOn, errFwdTunToDevOff error
	errFwdDevToTunOn, errFwdDevToTunOff error
	errClamp                            error

	// artificial delay inside methods to magnify races
	delay time.Duration
}

func (s *stubNF) enter() {
	if atomic.AddInt32(&s.active, 1) != 1 {
		atomic.StoreInt32(&s.seenConcurrent, 1)
	}
	if d := s.delay; d > 0 {
		time.Sleep(d)
	}
}
func (s *stubNF) leave() { atomic.AddInt32(&s.active, -1) }

func (s *stubNF) EnableDevMasquerade(string) error {
	s.enter()
	defer s.leave()
	atomic.AddInt32(&s.cMasqOn, 1)
	return s.errMasqOn
}
func (s *stubNF) DisableDevMasquerade(string) error {
	s.enter()
	defer s.leave()
	atomic.AddInt32(&s.cMasqOff, 1)
	return s.errMasqOff
}
func (s *stubNF) EnableForwardingFromTunToDev(string, string) error {
	s.enter()
	defer s.leave()
	atomic.AddInt32(&s.cFwdTunToDevOn, 1)
	return s.errFwdTunToDevOn
}
func (s *stubNF) DisableForwardingFromTunToDev(string, string) error {
	s.enter()
	defer s.leave()
	atomic.AddInt32(&s.cFwdTunToDevOff, 1)
	return s.errFwdTunToDevOff
}
func (s *stubNF) EnableForwardingFromDevToTun(string, string) error {
	s.enter()
	defer s.leave()
	atomic.AddInt32(&s.cFwdDevToTunOn, 1)
	return s.errFwdDevToTunOn
}
func (s *stubNF) DisableForwardingFromDevToTun(string, string) error {
	s.enter()
	defer s.leave()
	atomic.AddInt32(&s.cFwdDevToTunOff, 1)
	return s.errFwdDevToTunOff
}
func (s *stubNF) ConfigureMssClamping() error {
	s.enter()
	defer s.leave()
	atomic.AddInt32(&s.cClamp, 1)
	return s.errClamp
}

// Compile-time contract: decorator must implement the interface.
var _ application.Netfilter = (*nftables.SynchronizedNetfilter)(nil)

func TestDecorator_Delegates_AllMethods(t *testing.T) {
	base := &stubNF{}
	d := nftables.NewSynchronizedNetfilter(base)

	if err := d.EnableDevMasquerade("eth0"); err != nil {
		t.Fatalf("EnableDevMasquerade: %v", err)
	}
	if err := d.DisableDevMasquerade("eth0"); err != nil {
		t.Fatalf("DisableDevMasquerade: %v", err)
	}
	if err := d.EnableForwardingFromTunToDev("tun0", "eth0"); err != nil {
		t.Fatalf("EnableForwardingFromTunToDev: %v", err)
	}
	if err := d.DisableForwardingFromTunToDev("tun0", "eth0"); err != nil {
		t.Fatalf("DisableForwardingFromTunToDev: %v", err)
	}
	if err := d.EnableForwardingFromDevToTun("tun0", "eth0"); err != nil {
		t.Fatalf("EnableForwardingFromDevToTun: %v", err)
	}
	if err := d.DisableForwardingFromDevToTun("tun0", "eth0"); err != nil {
		t.Fatalf("DisableForwardingFromDevToTun: %v", err)
	}
	if err := d.ConfigureMssClamping(); err != nil {
		t.Fatalf("ConfigureMssClamping: %v", err)
	}

	if base.cMasqOn != 1 || base.cMasqOff != 1 ||
		base.cFwdTunToDevOn != 1 || base.cFwdTunToDevOff != 1 ||
		base.cFwdDevToTunOn != 1 || base.cFwdDevToTunOff != 1 ||
		base.cClamp != 1 {
		t.Fatalf("unexpected counters: %+v", base)
	}
}

func TestDecorator_PropagatesErrors(t *testing.T) {
	base := &stubNF{
		errMasqOn:         errors.New("e1"),
		errMasqOff:        errors.New("e2"),
		errFwdTunToDevOn:  errors.New("e3"),
		errFwdTunToDevOff: errors.New("e4"),
		errFwdDevToTunOn:  errors.New("e5"),
		errFwdDevToTunOff: errors.New("e6"),
		errClamp:          errors.New("e7"),
	}
	d := nftables.NewSynchronizedNetfilter(base)

	cases := []struct {
		name string
		fn   func() error
		want string
	}{
		{"MasqOn", func() error { return d.EnableDevMasquerade("eth0") }, "e1"},
		{"MasqOff", func() error { return d.DisableDevMasquerade("eth0") }, "e2"},
		{"FwdTunToDevOn", func() error { return d.EnableForwardingFromTunToDev("tun0", "eth0") }, "e3"},
		{"FwdTunToDevOff", func() error { return d.DisableForwardingFromTunToDev("tun0", "eth0") }, "e4"},
		{"FwdDevToTunOn", func() error { return d.EnableForwardingFromDevToTun("tun0", "eth0") }, "e5"},
		{"FwdDevToTunOff", func() error { return d.DisableForwardingFromDevToTun("tun0", "eth0") }, "e6"},
		{"Clamp", func() error { return d.ConfigureMssClamping() }, "e7"},
	}
	for _, tc := range cases {
		if err := tc.fn(); err == nil || err.Error() != tc.want {
			t.Fatalf("%s: expected %q, got %v", tc.name, tc.want, err)
		}
	}
}

func TestDecorator_SerializesUnderLoad(t *testing.T) {
	base := &stubNF{delay: 200 * time.Microsecond}
	d := nftables.NewSynchronizedNetfilter(base)

	var wg sync.WaitGroup
	work := []func(){
		func() { _ = d.EnableDevMasquerade("eth0") },
		func() { _ = d.DisableDevMasquerade("eth0") },
		func() { _ = d.EnableForwardingFromTunToDev("tun0", "eth0") },
		func() { _ = d.DisableForwardingFromTunToDev("tun0", "eth0") },
		func() { _ = d.EnableForwardingFromDevToTun("tun0", "eth0") },
		func() { _ = d.DisableForwardingFromDevToTun("tun0", "eth0") },
		func() { _ = d.ConfigureMssClamping() },
	}

	const N = 200
	wg.Add(N)
	for i := 0; i < N; i++ {
		fn := work[i%len(work)]
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	wg.Wait()

	if atomic.LoadInt32(&base.seenConcurrent) != 0 {
		t.Fatalf("underlying Netfilter saw concurrent access (decorator failed to serialize calls)")
	}

	// total number of calls must equal N
	total := base.cMasqOn + base.cMasqOff +
		base.cFwdTunToDevOn + base.cFwdTunToDevOff +
		base.cFwdDevToTunOn + base.cFwdDevToTunOff +
		base.cClamp
	if int(total) != N {
		t.Fatalf("expected %d total calls, got %d", N, int(total))
	}
}
