package handshake

import (
	"bytes"
	"crypto/sha256"
	"io"
	"testing"

	"golang.org/x/crypto/hkdf"
)

// Ensure the factory implements the interface
func TestDefaultSessionIdReaderFactory_ImplementsInterface(t *testing.T) {
	var _ SessionIdReaderFactory = NewDefaultSessionIdReader(
		[]byte("i"), // info
		[]byte("s"), // secret
		[]byte("s"), // salt
	)
}

// helper to read exactly n bytes or fail
func mustRead(t *testing.T, r io.Reader, n int) []byte {
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatalf("ReadFull error: %v", err)
	}
	return buf
}

// Compare factory output to direct HKDF
func TestDefaultSessionIdReaderFactory_Consistency(t *testing.T) {
	info := []byte("info")
	secret := []byte("secret")
	salt := []byte("salt")
	f := NewDefaultSessionIdReader(info, secret, salt)

	// direct HKDF
	direct := hkdf.New(sha256.New, secret, salt, info)
	want := mustRead(t, direct, 32)

	// via factory
	got := mustRead(t, f.NewReader(), 32)
	if !bytes.Equal(want, got) {
		t.Errorf("factory output = %x, want %x", got, want)
	}

	// second call should be the same
	again := mustRead(t, f.NewReader(), 32)
	if !bytes.Equal(want, again) {
		t.Errorf("factory repeated output = %x, want %x", again, want)
	}
}

// Changing any parameter must change output
func TestDefaultSessionIdReaderFactory_DifferentParameters(t *testing.T) {
	base := NewDefaultSessionIdReader([]byte("info"), []byte("secret"), []byte("salt"))
	a := mustRead(t, base.NewReader(), 32)

	// info
	fInfo := NewDefaultSessionIdReader([]byte("INFO"), []byte("secret"), []byte("salt"))
	b := mustRead(t, fInfo.NewReader(), 32)
	if bytes.Equal(a, b) {
		t.Error("outputs equal when info changed")
	}

	// secret
	fSec := NewDefaultSessionIdReader([]byte("info"), []byte("SECRET"), []byte("salt"))
	c := mustRead(t, fSec.NewReader(), 32)
	if bytes.Equal(a, c) {
		t.Error("outputs equal when secret changed")
	}

	// salt
	fSalt := NewDefaultSessionIdReader([]byte("info"), []byte("secret"), []byte("SALT"))
	d := mustRead(t, fSalt.NewReader(), 32)
	if bytes.Equal(a, d) {
		t.Error("outputs equal when salt changed")
	}
}
