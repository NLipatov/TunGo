package sysctl

import (
	"errors"
	"testing"
)

// sysctlMockCommander implements PAL.Commander for testing sysctl.Wrapper.
type sysctlMockCommander struct {
	// If non-nil, return these bytes and error for any CombinedOutput call.
	output []byte
	err    error
}

func (m *sysctlMockCommander) Output(_ string, _ ...string) ([]byte, error) {
	return m.output, m.err
}

func (m *sysctlMockCommander) CombinedOutput(_ string, _ ...string) ([]byte, error) {
	return m.output, m.err
}

func TestNetIpv4IpForward_Success(t *testing.T) {
	expected := []byte("net.ipv4.ip_forward = 1\n")
	mock := &sysctlMockCommander{output: expected, err: nil}
	w := NewWrapper(mock)

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
	mock := &sysctlMockCommander{output: nil, err: mockErr}
	w := NewWrapper(mock)

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
	mock := &sysctlMockCommander{output: expected, err: nil}
	w := NewWrapper(mock)

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
	mock := &sysctlMockCommander{output: nil, err: mockErr}
	w := NewWrapper(mock)

	out, err := w.WNetIpv4IpForward()
	if !errors.Is(err, mockErr) {
		t.Fatalf("got error %v, want %v", err, mockErr)
	}
	if out != nil {
		t.Errorf("expected nil output on error, got %q", out)
	}
}
