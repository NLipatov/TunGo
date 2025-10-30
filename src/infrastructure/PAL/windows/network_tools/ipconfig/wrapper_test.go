package ipconfig

import (
	"errors"
	"testing"
)

type mockCommander struct {
	calledName string
	calledArgs []string
	output     []byte
	err        error
}

func (m *mockCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	m.calledName = name
	m.calledArgs = args
	return m.output, m.err
}

func (m *mockCommander) Output(_ string, _ ...string) ([]byte, error) {
	panic("Output not used in tests")
}

func (m *mockCommander) Run(_ string, _ ...string) error {
	panic("Run not used in tests")
}

func TestFlushDNS_Success(t *testing.T) {
	mock := &mockCommander{
		output: []byte("Successfully flushed the DNS Resolver Cache."),
		err:    nil,
	}
	w := NewWrapper(mock)
	err := w.FlushDNS()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if mock.calledName != "ipconfig" {
		t.Fatalf("expected command 'ipconfig', got: %s", mock.calledName)
	}
	if len(mock.calledArgs) != 1 || mock.calledArgs[0] != "/flushdns" {
		t.Fatalf("expected args ['/flushdns'], got: %v", mock.calledArgs)
	}
}

func TestFlushDNS_Error(t *testing.T) {
	mock := &mockCommander{
		output: []byte("Access is denied."),
		err:    errors.New("exit status 1"),
	}
	w := NewWrapper(mock)
	err := w.FlushDNS()
	expected := "flushdns: Access is denied."
	if err == nil || err.Error() != expected {
		t.Fatalf("expected error %q, got: %v", expected, err)
	}
}
