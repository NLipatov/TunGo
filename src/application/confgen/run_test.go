package confgen

import (
	"errors"
	"strings"
	"testing"
)

func TestPrepareServerKeys_ExistingKeys(t *testing.T) {
	manager := &mockMgr{
		cfg: validCfg(),
	}
	manager.cfg.X25519PublicKey = make([]byte, 32)
	manager.cfg.X25519PrivateKey = make([]byte, 32)

	if err := prepareServerKeys(manager); err != nil {
		t.Fatalf("prepareServerKeys() error = %v", err)
	}
	if manager.injectCalls != 0 {
		t.Fatalf("expected existing keys to skip injection, got %d inject calls", manager.injectCalls)
	}
}

func TestNewServerConfigurationManager(t *testing.T) {
	manager, err := newServerConfigurationManager()
	if err != nil {
		t.Fatalf("newServerConfigurationManager() error = %v", err)
	}
	if manager == nil {
		t.Fatal("expected manager")
	}
}

func TestPrepareServerKeys_InjectError(t *testing.T) {
	manager := &mockMgr{
		cfg:       validCfg(),
		injectErr: errors.New("inject failed"),
	}

	err := prepareServerKeys(manager)
	if err == nil || !strings.Contains(err.Error(), "could not prepare keys") {
		t.Fatalf("expected wrapped key preparation error, got %v", err)
	}
	if manager.injectCalls != 1 {
		t.Fatalf("expected generated keys to be injected once, got %d", manager.injectCalls)
	}
}
