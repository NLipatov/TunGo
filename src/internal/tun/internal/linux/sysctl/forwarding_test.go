package sysctl

import (
	"errors"
	"testing"
)

// sysctlMockRunner records commands for forwarding tests.
type sysctlMockRunner struct {
	// If non-nil, return these bytes and error for any CombinedOutput call.
	output []byte
	err    error
}

func (m *sysctlMockRunner) Run(_ string, _ ...string) error {
	panic("not implemented")
}

func (m *sysctlMockRunner) Output(_ string, _ ...string) ([]byte, error) {
	return m.output, m.err
}

func (m *sysctlMockRunner) CombinedOutput(_ string, _ ...string) ([]byte, error) {
	return m.output, m.err
}

func TestNetIpv4IpForward_Success(t *testing.T) {
	expected := []byte("net.ipv4.ip_forward = 1\n")
	mock := &sysctlMockRunner{output: expected, err: nil}
	w := New(mock)

	out, err := w.NetIpv4IpForward()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(expected) {
		t.Errorf("got %q, want %q", out, expected)
	}
}

func TestNetIpv4IpForward_Error(t *testing.T) {
	mockErr := errors.New("sysctl failed")
	mock := &sysctlMockRunner{output: nil, err: mockErr}
	w := New(mock)

	out, err := w.NetIpv4IpForward()
	if !errors.Is(err, mockErr) {
		t.Fatalf("got error %v, want %v", err, mockErr)
	}
	if out != nil {
		t.Errorf("expected nil output on error, got %q", out)
	}
}

func TestWNetIpv4IpForward_Success(t *testing.T) {
	expected := []byte("net.ipv4.ip_forward = 1\n")
	mock := &sysctlMockRunner{output: expected, err: nil}
	w := New(mock)

	out, err := w.WNetIpv4IpForward()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(expected) {
		t.Errorf("got %q, want %q", out, expected)
	}
}

func TestWNetIpv4IpForward_Error(t *testing.T) {
	mockErr := errors.New("cannot write")
	mock := &sysctlMockRunner{output: nil, err: mockErr}
	w := New(mock)

	out, err := w.WNetIpv4IpForward()
	if !errors.Is(err, mockErr) {
		t.Fatalf("got error %v, want %v", err, mockErr)
	}
	if out != nil {
		t.Errorf("expected nil output on error, got %q", out)
	}
}

func TestNetIpv6ConfAllForwarding_Success(t *testing.T) {
	expected := []byte("net.ipv6.conf.all.forwarding = 1\n")
	mock := &sysctlMockRunner{output: expected, err: nil}
	w := New(mock)

	out, err := w.NetIpv6ConfAllForwarding()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(expected) {
		t.Errorf("got %q, want %q", out, expected)
	}
}

func TestNetIpv6ConfAllForwarding_Error(t *testing.T) {
	mockErr := errors.New("sysctl failed")
	mock := &sysctlMockRunner{output: nil, err: mockErr}
	w := New(mock)

	out, err := w.NetIpv6ConfAllForwarding()
	if !errors.Is(err, mockErr) {
		t.Fatalf("got error %v, want %v", err, mockErr)
	}
	if out != nil {
		t.Errorf("expected nil output on error, got %q", out)
	}
}

func TestWNetIpv6ConfAllForwarding_Success(t *testing.T) {
	expected := []byte("net.ipv6.conf.all.forwarding = 1\n")
	mock := &sysctlMockRunner{output: expected, err: nil}
	w := New(mock)

	out, err := w.WNetIpv6ConfAllForwarding()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(expected) {
		t.Errorf("got %q, want %q", out, expected)
	}
}

func TestWNetIpv6ConfAllForwarding_Error(t *testing.T) {
	mockErr := errors.New("cannot write")
	mock := &sysctlMockRunner{output: nil, err: mockErr}
	w := New(mock)

	out, err := w.WNetIpv6ConfAllForwarding()
	if !errors.Is(err, mockErr) {
		t.Fatalf("got error %v, want %v", err, mockErr)
	}
	if out != nil {
		t.Errorf("expected nil output on error, got %q", out)
	}
}
