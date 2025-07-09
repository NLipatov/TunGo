package handshake

import (
	"bytes"
	"testing"
)

func TestObfuscator_RoundTrip(t *testing.T) {
	orig := []byte("hello world")
	obf, err := (Obfuscator{}).Obfuscate(orig)
	if err != nil {
		t.Fatalf("obfuscate error: %v", err)
	}
	plain, ok, err := (Obfuscator{}).Deobfuscate(obf)
	if err != nil {
		t.Fatalf("deobfuscate error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for obfuscated data")
	}
	if !bytes.Equal(plain, orig) {
		t.Errorf("roundtrip mismatch: got %q want %q", plain, orig)
	}
}

func TestObfuscator_Plain(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	out, ok, err := (Obfuscator{}).Deobfuscate(data)
	if err != nil {
		t.Fatalf("deobfuscate plain error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for plain data")
	}
	if !bytes.Equal(out, data) {
		t.Errorf("data changed")
	}
}
