package handshake

import (
	"errors"
	"testing"

	"tungo/settings/client_configuration"
)

type fakeCM struct {
	key []byte
	err error
}

func (f *fakeCM) Configuration() (*client_configuration.Configuration, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &client_configuration.Configuration{Ed25519PublicKey: f.key}, nil
}

func TestServerEd25519PublicKey_Success(t *testing.T) {
	want := []byte{9, 8, 7}
	cm := &fakeCM{key: want}
	c := NewDefaultClientConf(cm)
	got, err := c.ServerEd25519PublicKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestServerEd25519PublicKey_Error(t *testing.T) {
	sentinel := errors.New("fail")
	cm := &fakeCM{err: sentinel}
	c := NewDefaultClientConf(cm)
	_, err := c.ServerEd25519PublicKey()
	if err != sentinel {
		t.Errorf("expected error %v, got %v", sentinel, err)
	}
}
