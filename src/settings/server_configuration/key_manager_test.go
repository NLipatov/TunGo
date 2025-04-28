package server_configuration

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"testing"
)

type fakeStore struct {
	injectCalls int
	lastPub     ed25519.PublicKey
	lastPriv    ed25519.PrivateKey
}

func (f *fakeStore) Configuration() (*Configuration, error) {
	return &Configuration{}, nil
}
func (f *fakeStore) IncrementClientCounter() error {
	return nil
}
func (f *fakeStore) InjectEdKeys(pub ed25519.PublicKey, priv ed25519.PrivateKey) error {
	f.injectCalls++
	f.lastPub = pub
	f.lastPriv = priv
	return nil
}

func TestPrepareKeys_SkipsWhenConfigHasKeys(t *testing.T) {
	cfg := &Configuration{
		Ed25519PublicKey:  []byte{1, 2, 3},
		Ed25519PrivateKey: []byte{4, 5, 6},
	}
	store := &fakeStore{}
	km := NewEd25519KeyManager(cfg, store)

	if err := km.PrepareKeys(); err != nil {
		t.Fatalf("PrepareKeys returned error: %v", err)
	}
	if store.injectCalls != 0 {
		t.Errorf("expected no inject calls, got %d", store.injectCalls)
	}
}

func TestPrepareKeys_UsesEnvKeys(t *testing.T) {
	// generate a real key pair and set env
	pub, priv, _ := ed25519.GenerateKey(nil)
	os.Setenv(pubEnvVar, base64.StdEncoding.EncodeToString(pub))
	os.Setenv(privEnvVar, base64.StdEncoding.EncodeToString(priv))
	defer os.Unsetenv(pubEnvVar)
	defer os.Unsetenv(privEnvVar)

	cfg := &Configuration{} // empty config
	store := &fakeStore{}
	km := NewEd25519KeyManager(cfg, store)

	if err := km.PrepareKeys(); err != nil {
		t.Fatalf("PrepareKeys returned error: %v", err)
	}
	if store.injectCalls != 1 {
		t.Fatalf("expected 1 inject call, got %d", store.injectCalls)
	}
	if !ed25519.PublicKey(store.lastPub).Equal(pub) {
		t.Error("injected public key does not match env value")
	}
}

func TestPrepareKeys_GeneratesWhenEnvMissing(t *testing.T) {
	os.Unsetenv(pubEnvVar)
	os.Unsetenv(privEnvVar)

	cfg := &Configuration{}
	store := &fakeStore{}
	km := NewEd25519KeyManager(cfg, store)

	if err := km.PrepareKeys(); err != nil {
		t.Fatalf("PrepareKeys returned error: %v", err)
	}
	if store.injectCalls != 1 {
		t.Fatalf("expected 1 inject call, got %d", store.injectCalls)
	}
	// keys should be non-empty
	if len(store.lastPub) == 0 || len(store.lastPriv) == 0 {
		t.Error("generated keys are empty")
	}
}

func TestTryEnvKeys_ErrorsOnInvalidBase64(t *testing.T) {
	// set invalid base64 for public, valid for private
	os.Setenv(pubEnvVar, "!!!not-base64!!!")
	os.Setenv(privEnvVar, base64.StdEncoding.EncodeToString([]byte{1, 2, 3}))
	defer os.Unsetenv(pubEnvVar)
	defer os.Unsetenv(privEnvVar)

	cfg := &Configuration{}
	store := &fakeStore{}
	km := NewEd25519KeyManager(cfg, store)

	// call tryEnvKeys via PrepareKeys: errors, then generateAndStore should run
	if err := km.PrepareKeys(); err != nil {
		t.Fatalf("PrepareKeys returned error: %v", err)
	}
	// since env decode failed, fallback to generate -> injectCalls == 1
	if store.injectCalls != 1 {
		t.Errorf("expected fallback inject call, got %d", store.injectCalls)
	}
}
