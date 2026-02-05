package server

import (
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"
)

// Compile-time check tath KeyManager implement KeyManager interface.
var _ KeyManager = &X25519KeyManager{}

// ---------- Mocks ----------

// mockConfigurationManager fakes ServerConfigurationManager for tests.
type mockConfigurationManager struct {
	cfg         *Configuration
	cfgErr      error
	injectErr   error
	injectCalls int
	lastPub     []byte
	lastPriv    []byte
}

func (m *mockConfigurationManager) Configuration() (*Configuration, error) {
	return m.cfg, m.cfgErr
}
func (m *mockConfigurationManager) IncrementClientCounter() error { return nil } // unused here

func (m *mockConfigurationManager) InjectX25519Keys(pub, priv []byte) error {
	m.injectCalls++
	m.lastPub = append([]byte(nil), pub...)
	m.lastPriv = append([]byte(nil), priv...)
	if m.injectErr != nil {
		return m.injectErr
	}
	// Simulate persistence into configuration.
	if m.cfg != nil {
		m.cfg.X25519PublicKey = pub
		m.cfg.X25519PrivateKey = priv
	}
	return nil
}

func (m *mockConfigurationManager) AddAllowedPeer(_ AllowedPeer) error {
	return nil
}

func (m *mockConfigurationManager) InvalidateCache() {}

// ---------- Helpers ----------

func newKM(manager *mockConfigurationManager) *X25519KeyManager {
	return &X25519KeyManager{configurationManager: manager}
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
	pub := make([]byte, 32)
	priv := make([]byte, 32)
	manager := &mockConfigurationManager{
		cfg: &Configuration{
			X25519PublicKey:  pub,
			X25519PrivateKey: priv,
		},
	}
	km := newKM(manager)

	if err := km.PrepareKeys(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if manager.injectCalls != 0 {
		t.Fatalf("should not inject when config already has keys")
	}
}

func TestPrepareKeys_EnvKeys_Valid_Injection(t *testing.T) {
	manager := &mockConfigurationManager{cfg: &Configuration{}}
	km := newKM(manager)

	pub := make([]byte, 32)
	priv := make([]byte, 32)
	mustSetenv(t, publicKeyEnvVar, base64.StdEncoding.EncodeToString(pub))
	mustSetenv(t, privateKeyEnvVar, base64.StdEncoding.EncodeToString(priv))

	if err := km.PrepareKeys(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if manager.injectCalls != 1 {
		t.Fatalf("expected 1 inject call, got %d", manager.injectCalls)
	}
	if len(manager.lastPub) != 32 || len(manager.lastPriv) != 32 {
		t.Fatalf("wrong sizes: pub=%d priv=%d", len(manager.lastPub), len(manager.lastPriv))
	}
}

func TestPrepareKeys_EnvMissing_FallsBackToGenerate(t *testing.T) {
	manager := &mockConfigurationManager{cfg: &Configuration{}}
	km := newKM(manager)

	_ = os.Unsetenv(publicKeyEnvVar)
	_ = os.Unsetenv(privateKeyEnvVar)

	if err := km.PrepareKeys(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if manager.injectCalls != 1 {
		t.Fatalf("generate path should inject once, got %d", manager.injectCalls)
	}
	if len(manager.lastPub) != 32 || len(manager.lastPriv) != 32 {
		t.Fatalf("generated sizes mismatch")
	}
}

func TestPrepareKeys_EnvInvalidBase64_FallsBackToGenerate(t *testing.T) {
	manager := &mockConfigurationManager{cfg: &Configuration{}}
	km := newKM(manager)

	mustSetenv(t, publicKeyEnvVar, "!!!not-base64!!!")
	mustSetenv(t, privateKeyEnvVar, "!!!still-not-base64!!!")

	if err := km.PrepareKeys(); err != nil {
		t.Fatalf("should still succeed by generating: %v", err)
	}
	if manager.injectCalls != 1 {
		t.Fatalf("expected 1 inject call (generate), got %d", manager.injectCalls)
	}
}

func TestPrepareKeys_Generate_InjectError_Propagates(t *testing.T) {
	manager := &mockConfigurationManager{
		cfg:       &Configuration{},
		injectErr: errors.New("inject-fail"),
	}
	km := newKM(manager)

	_ = os.Unsetenv(publicKeyEnvVar)
	_ = os.Unsetenv(privateKeyEnvVar)

	err := km.PrepareKeys()
	if err == nil || !strings.Contains(err.Error(), "inject-fail") {
		t.Fatalf("want inject-fail, got %v", err)
	}
}

// ---------- Tests: keysAreInConfiguration ----------

func Test_keysAreInConfiguration_ReadError(t *testing.T) {
	manager := &mockConfigurationManager{cfgErr: errors.New("read-fail")}
	km := newKM(manager)

	ok, err := km.keysAreInConfiguration()
	if ok || err == nil {
		t.Fatalf("expected (false, error), got (%v, %v)", ok, err)
	}
}

func Test_keysAreInConfiguration_LengthValidation(t *testing.T) {
	// wrong lengths -> false
	manager1 := &mockConfigurationManager{cfg: &Configuration{
		X25519PublicKey:  make([]byte, 10),
		X25519PrivateKey: make([]byte, 20),
	}}
	km1 := newKM(manager1)
	ok, err := km1.keysAreInConfiguration()
	if ok || err != nil {
		t.Fatalf("want (false,nil) for wrong lengths, got (%v,%v)", ok, err)
	}

	// correct lengths -> true
	manager2 := &mockConfigurationManager{cfg: &Configuration{
		X25519PublicKey:  make([]byte, 32),
		X25519PrivateKey: make([]byte, 32),
	}}
	km2 := newKM(manager2)
	ok, err = km2.keysAreInConfiguration()
	if !ok || err != nil {
		t.Fatalf("want (true,nil), got (%v,%v)", ok, err)
	}
}

// ---------- Tests: keysAreInEnvVariables ----------

func Test_keysAreInEnvVariables_Empty_NoErrorFalse(t *testing.T) {
	manager := &mockConfigurationManager{cfg: &Configuration{}}
	km := newKM(manager)

	_ = os.Unsetenv(publicKeyEnvVar)
	_ = os.Unsetenv(privateKeyEnvVar)

	ok, err := km.keysAreInEnvVariables()
	if ok || err != nil {
		t.Fatalf("want (false, nil), got (%v, %v)", ok, err)
	}
}

func Test_keysAreInEnvVariables_BadPublicBase64(t *testing.T) {
	manager := &mockConfigurationManager{cfg: &Configuration{}}
	km := newKM(manager)

	mustSetenv(t, publicKeyEnvVar, "!!!bad!!!")
	mustSetenv(t, privateKeyEnvVar, base64.StdEncoding.EncodeToString(make([]byte, 32)))

	ok, err := km.keysAreInEnvVariables()
	if ok || err == nil || !strings.Contains(err.Error(), "failed to decode public key") {
		t.Fatalf("expected public decode error, got (%v, %v)", ok, err)
	}
}

func Test_keysAreInEnvVariables_BadPrivateBase64(t *testing.T) {
	manager := &mockConfigurationManager{cfg: &Configuration{}}
	km := newKM(manager)

	mustSetenv(t, publicKeyEnvVar, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	mustSetenv(t, privateKeyEnvVar, "!!!bad!!!")

	ok, err := km.keysAreInEnvVariables()
	if ok || err == nil || !strings.Contains(err.Error(), "failed to decode private key") {
		t.Fatalf("expected private decode error, got (%v, %v)", ok, err)
	}
}

func Test_keysAreInEnvVariables_InjectError(t *testing.T) {
	manager := &mockConfigurationManager{
		cfg:       &Configuration{},
		injectErr: errors.New("inject-fail"),
	}
	km := newKM(manager)

	mustSetenv(t, publicKeyEnvVar, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	mustSetenv(t, privateKeyEnvVar, base64.StdEncoding.EncodeToString(make([]byte, 32)))

	ok, err := km.keysAreInEnvVariables()
	if ok || err == nil || !strings.Contains(err.Error(), "failed to inject X25519 key pair") {
		t.Fatalf("want inject error, got (%v, %v)", ok, err)
	}
}

func Test_keysAreInEnvVariables_Valid(t *testing.T) {
	manager := &mockConfigurationManager{cfg: &Configuration{}}
	km := newKM(manager)

	mustSetenv(t, publicKeyEnvVar, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	mustSetenv(t, privateKeyEnvVar, base64.StdEncoding.EncodeToString(make([]byte, 32)))

	ok, err := km.keysAreInEnvVariables()
	if !ok || err != nil {
		t.Fatalf("want (true, nil), got (%v, %v)", ok, err)
	}
	if manager.injectCalls != 1 {
		t.Fatalf("expected inject once, got %d", manager.injectCalls)
	}
}
