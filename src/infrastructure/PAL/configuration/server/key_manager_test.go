package server

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"
)

// Compile-time check tath KeyManager implement KeyManager interface.
var _ KeyManager = &Ed25519KeyManager{}

// ---------- Mocks (prefixed with Ed25519KeyManager...) ----------

// Ed25519KeyManagerMockStore fakes ServerConfigurationManager for tests.
type Ed25519KeyManagerMockStore struct {
	cfg         *Configuration
	cfgErr      error
	injectErr   error
	injectCalls int
	lastPub     []byte
	lastPriv    []byte
}

func (m *Ed25519KeyManagerMockStore) Configuration() (*Configuration, error) {
	return m.cfg, m.cfgErr
}
func (m *Ed25519KeyManagerMockStore) IncrementClientCounter() error { return nil } // unused here

// IMPORTANT:
// This mock uses the ed25519.PublicKey/PrivateKey signature to mirror the usual
// ServerConfigurationManager contract. If your real interface uses []byte, adjust here.
func (m *Ed25519KeyManagerMockStore) InjectEdKeys(pub ed25519.PublicKey, priv ed25519.PrivateKey) error {
	m.injectCalls++
	m.lastPub = append([]byte(nil), pub...)
	m.lastPriv = append([]byte(nil), priv...)
	if m.injectErr != nil {
		return m.injectErr
	}
	// Simulate persistence into configuration.
	if m.cfg != nil {
		m.cfg.Ed25519PublicKey = pub
		m.cfg.Ed25519PrivateKey = priv
	}
	return nil
}

// ---------- Helpers ----------

func newKM(store *Ed25519KeyManagerMockStore) *Ed25519KeyManager {
	return &Ed25519KeyManager{configurationManager: store}
}

func mustSetenv(t *testing.T, k, v string) {
	t.Helper()
	if err := os.Setenv(k, v); err != nil {
		t.Fatalf("setenv %s: %v", k, err)
	}
	t.Cleanup(func() { _ = os.Unsetenv(k) })
}

// ---------- Tests: PrepareKeys high-level paths ----------

func TestPrepareKeys_ConfigAlreadyHasValidKeys_NoOp(t *testing.T) {
	pub := make([]byte, ed25519.PublicKeySize)
	priv := make([]byte, ed25519.PrivateKeySize)
	store := &Ed25519KeyManagerMockStore{
		cfg: &Configuration{
			Ed25519PublicKey:  pub,
			Ed25519PrivateKey: priv,
		},
	}
	km := newKM(store)

	if err := km.PrepareKeys(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.injectCalls != 0 {
		t.Fatalf("should not inject when config already has keys")
	}
}

func TestPrepareKeys_EnvKeys_Valid_Injection(t *testing.T) {
	store := &Ed25519KeyManagerMockStore{cfg: &Configuration{}}
	km := newKM(store)

	pub := make([]byte, ed25519.PublicKeySize)
	priv := make([]byte, ed25519.PrivateKeySize)
	mustSetenv(t, publicKeyEnvVar, base64.StdEncoding.EncodeToString(pub))
	mustSetenv(t, privateKeyEnvVar, base64.StdEncoding.EncodeToString(priv))

	if err := km.PrepareKeys(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.injectCalls != 1 {
		t.Fatalf("expected 1 inject call, got %d", store.injectCalls)
	}
	if len(store.lastPub) != ed25519.PublicKeySize || len(store.lastPriv) != ed25519.PrivateKeySize {
		t.Fatalf("wrong sizes: pub=%d priv=%d", len(store.lastPub), len(store.lastPriv))
	}
}

func TestPrepareKeys_EnvMissing_FallsBackToGenerate(t *testing.T) {
	store := &Ed25519KeyManagerMockStore{cfg: &Configuration{}}
	km := newKM(store)

	_ = os.Unsetenv(publicKeyEnvVar)
	_ = os.Unsetenv(privateKeyEnvVar)

	if err := km.PrepareKeys(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if store.injectCalls != 1 {
		t.Fatalf("generate path should inject once, got %d", store.injectCalls)
	}
	if len(store.lastPub) != ed25519.PublicKeySize || len(store.lastPriv) != ed25519.PrivateKeySize {
		t.Fatalf("generated sizes mismatch")
	}
}

func TestPrepareKeys_EnvInvalidBase64_FallsBackToGenerate(t *testing.T) {
	store := &Ed25519KeyManagerMockStore{cfg: &Configuration{}}
	km := newKM(store)

	mustSetenv(t, publicKeyEnvVar, "!!!not-base64!!!")
	mustSetenv(t, privateKeyEnvVar, "!!!still-not-base64!!!")

	if err := km.PrepareKeys(); err != nil {
		t.Fatalf("should still succeed by generating: %v", err)
	}
	if store.injectCalls != 1 {
		t.Fatalf("expected 1 inject call (generate), got %d", store.injectCalls)
	}
}

func TestPrepareKeys_Generate_InjectError_Propagates(t *testing.T) {
	store := &Ed25519KeyManagerMockStore{
		cfg:       &Configuration{},
		injectErr: errors.New("inject-fail"),
	}
	km := newKM(store)

	_ = os.Unsetenv(publicKeyEnvVar)
	_ = os.Unsetenv(privateKeyEnvVar)

	err := km.PrepareKeys()
	if err == nil || !strings.Contains(err.Error(), "inject-fail") {
		t.Fatalf("want inject-fail, got %v", err)
	}
}

// ---------- Tests: keysAreInConfiguration ----------

func Test_keysAreInConfiguration_ReadError(t *testing.T) {
	store := &Ed25519KeyManagerMockStore{cfgErr: errors.New("read-fail")}
	km := newKM(store)

	ok, err := km.keysAreInConfiguration()
	if ok || err == nil {
		t.Fatalf("expected (false, error), got (%v, %v)", ok, err)
	}
}

func Test_keysAreInConfiguration_LengthValidation(t *testing.T) {
	// wrong lengths -> false
	store1 := &Ed25519KeyManagerMockStore{cfg: &Configuration{
		Ed25519PublicKey:  make([]byte, 10),
		Ed25519PrivateKey: make([]byte, 20),
	}}
	km1 := newKM(store1)
	ok, err := km1.keysAreInConfiguration()
	if ok || err != nil {
		t.Fatalf("want (false,nil) for wrong lengths, got (%v,%v)", ok, err)
	}

	// correct lengths -> true
	store2 := &Ed25519KeyManagerMockStore{cfg: &Configuration{
		Ed25519PublicKey:  make([]byte, ed25519.PublicKeySize),
		Ed25519PrivateKey: make([]byte, ed25519.PrivateKeySize),
	}}
	km2 := newKM(store2)
	ok, err = km2.keysAreInConfiguration()
	if !ok || err != nil {
		t.Fatalf("want (true,nil), got (%v,%v)", ok, err)
	}
}

// ---------- Tests: keysAreInEnvVariables ----------

func Test_keysAreInEnvVariables_Empty_NoErrorFalse(t *testing.T) {
	store := &Ed25519KeyManagerMockStore{cfg: &Configuration{}}
	km := newKM(store)

	_ = os.Unsetenv(publicKeyEnvVar)
	_ = os.Unsetenv(privateKeyEnvVar)

	ok, err := km.keysAreInEnvVariables()
	if ok || err != nil {
		t.Fatalf("want (false, nil), got (%v, %v)", ok, err)
	}
}

func Test_keysAreInEnvVariables_BadPublicBase64(t *testing.T) {
	store := &Ed25519KeyManagerMockStore{cfg: &Configuration{}}
	km := newKM(store)

	mustSetenv(t, publicKeyEnvVar, "!!!bad!!!")
	mustSetenv(t, privateKeyEnvVar, base64.StdEncoding.EncodeToString(make([]byte, ed25519.PrivateKeySize)))

	ok, err := km.keysAreInEnvVariables()
	if ok || err == nil || !strings.Contains(err.Error(), "failed to decode public key") {
		t.Fatalf("expected public decode error, got (%v, %v)", ok, err)
	}
}

func Test_keysAreInEnvVariables_BadPrivateBase64(t *testing.T) {
	store := &Ed25519KeyManagerMockStore{cfg: &Configuration{}}
	km := newKM(store)

	mustSetenv(t, publicKeyEnvVar, base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize)))
	mustSetenv(t, privateKeyEnvVar, "!!!bad!!!")

	ok, err := km.keysAreInEnvVariables()
	if ok || err == nil || !strings.Contains(err.Error(), "failed to decode private key") {
		t.Fatalf("expected private decode error, got (%v, %v)", ok, err)
	}
}

func Test_keysAreInEnvVariables_InjectError(t *testing.T) {
	store := &Ed25519KeyManagerMockStore{
		cfg:       &Configuration{},
		injectErr: errors.New("inject-fail"),
	}
	km := newKM(store)

	mustSetenv(t, publicKeyEnvVar, base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize)))
	mustSetenv(t, privateKeyEnvVar, base64.StdEncoding.EncodeToString(make([]byte, ed25519.PrivateKeySize)))

	ok, err := km.keysAreInEnvVariables()
	if ok || err == nil || !strings.Contains(err.Error(), "failed to inject Ed25519 key pair") {
		t.Fatalf("want inject error, got (%v, %v)", ok, err)
	}
}

func Test_keysAreInEnvVariables_Valid(t *testing.T) {
	store := &Ed25519KeyManagerMockStore{cfg: &Configuration{}}
	km := newKM(store)

	mustSetenv(t, publicKeyEnvVar, base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize)))
	mustSetenv(t, privateKeyEnvVar, base64.StdEncoding.EncodeToString(make([]byte, ed25519.PrivateKeySize)))

	ok, err := km.keysAreInEnvVariables()
	if !ok || err != nil {
		t.Fatalf("want (true, nil), got (%v, %v)", ok, err)
	}
	if store.injectCalls != 1 {
		t.Fatalf("expected inject once, got %d", store.injectCalls)
	}
}
