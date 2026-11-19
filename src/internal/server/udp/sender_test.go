//go:build linux

package udp

import (
	"errors"
	"testing"

	"tungo/internal/server/session"
)

type encryptFunc func([]byte) ([]byte, error)

func (f encryptFunc) Encrypt(data []byte) ([]byte, error) { return f(data) }

type writeFunc func([]byte) (int, error)

func (f writeFunc) Write(data []byte) (int, error) { return f(data) }

func TestPeerSender_ClassifiesErrors(t *testing.T) {
	cause := errors.New("cause")
	passthrough := encryptFunc(func(data []byte) ([]byte, error) { return data, nil })
	discard := writeFunc(func(data []byte) (int, error) { return len(data), nil })

	for _, tt := range []struct {
		name    string
		crypto  encryptFunc
		writer  writeFunc
		wantErr error
	}{
		{name: "success", crypto: passthrough, writer: discard},
		{
			name:    "encryption",
			crypto:  func([]byte) ([]byte, error) { return nil, cause },
			writer:  discard,
			wantErr: session.ErrEncryption,
		},
		{
			name:    "transport write",
			crypto:  passthrough,
			writer:  func([]byte) (int, error) { return 0, cause },
			wantErr: session.ErrTransportWrite,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := newPeerSender(tt.writer, tt.crypto).Send([]byte("packet"))
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Send error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) || !errors.Is(err, cause) {
				t.Fatalf("Send error = %v, want %v wrapping %v", err, tt.wantErr, cause)
			}
		})
	}
}
