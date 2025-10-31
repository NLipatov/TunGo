package route

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// WrapperMockCommander implements PAL.Commander for tests.
// Name is prefixed with the SUT ("Wrapper") per your convention.
type WrapperMockCommander struct {
	// captured call
	GotName string
	GotArgs []string

	// call counters (useful if you later add other calls)
	CallsCombined int
	CallsOutput   int
	CallsRun      int

	// configured result
	Out []byte
	Err error
}

func (m *WrapperMockCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	m.CallsCombined++
	m.GotName = name
	m.GotArgs = append([]string(nil), args...)
	return m.Out, m.Err
}

func (m *WrapperMockCommander) Output(_ string, _ ...string) ([]byte, error) {
	m.CallsOutput++
	return m.Out, m.Err
}

func (m *WrapperMockCommander) Run(_ string, _ ...string) error {
	m.CallsRun++
	return m.Err
}

func TestWrapper_RouteDelete_Success(t *testing.T) {
	mock := &WrapperMockCommander{Out: []byte("ok"), Err: nil}
	w := &Wrapper{commander: mock}

	ip := "1.2.3.4"
	if err := w.RouteDelete(ip); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if mock.CallsCombined != 1 {
		t.Fatalf("expected exactly 1 CombinedOutput call, got %d", mock.CallsCombined)
	}
	if mock.GotName != "route" {
		t.Fatalf("expected command 'route', got %q", mock.GotName)
	}
	wantArgs := []string{"delete", ip}
	if !reflect.DeepEqual(mock.GotArgs, wantArgs) {
		t.Fatalf("args mismatch: want %v, got %v", wantArgs, mock.GotArgs)
	}
}

func TestWrapper_RouteDelete_Error_PropagatesMessage(t *testing.T) {
	mock := &WrapperMockCommander{Out: []byte("bad params"), Err: errors.New("exit status 1")}
	w := &Wrapper{commander: mock}

	err := w.RouteDelete("10.0.0.1")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "RouteDelete error") {
		t.Fatalf("expected error message to contain 'RouteDelete error', got: %q", msg)
	}
	if !strings.Contains(msg, "bad params") {
		t.Fatalf("expected error message to contain captured output, got: %q", msg)
	}
	if !strings.Contains(msg, "exit status 1") {
		t.Fatalf("expected original error to be included, got: %q", msg)
	}
	if mock.CallsCombined != 1 {
		t.Fatalf("expected exactly 1 CombinedOutput call, got %d", mock.CallsCombined)
	}
}

func TestWrapper_RouteDelete_ForwardsIPv6Verbatim(t *testing.T) {
	// Thin pass-through: ensure the IPv6-looking string is forwarded unchanged.
	mock := &WrapperMockCommander{Out: []byte("ok"), Err: nil}
	w := &Wrapper{commander: mock}

	host := "2001:db8::1"
	if err := w.RouteDelete(host); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	wantArgs := []string{"delete", host}
	if !reflect.DeepEqual(mock.GotArgs, wantArgs) {
		t.Fatalf("args mismatch: want %v, got %v", wantArgs, mock.GotArgs)
	}
}
