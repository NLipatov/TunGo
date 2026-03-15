package infrastructure

import "testing"

func TestRequireAdminPrivileges(t *testing.T) {
	hooks := Hooks{Geteuid: func() int { return 0 }}
	if err := RequireAdminPrivileges(hooks); err != nil {
		t.Fatalf("expected no error for root, got %v", err)
	}

	hooks.Geteuid = func() int { return 1000 }
	if err := RequireAdminPrivileges(hooks); err == nil {
		t.Fatal("expected error for non-root")
	}

	if err := RequireAdminPrivileges(Hooks{}); err == nil {
		t.Fatal("expected error when geteuid hook is unset")
	}
}
