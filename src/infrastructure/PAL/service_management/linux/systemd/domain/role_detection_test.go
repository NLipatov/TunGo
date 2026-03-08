package domain

import "testing"

func TestDetectUnitRole(t *testing.T) {
	if got := DetectUnitRole("ExecStart=tungo c\n"); got != UnitRoleClient {
		t.Fatalf("expected client role, got %q", got)
	}
	if got := DetectUnitRole("ExecStart=tungo s\n"); got != UnitRoleServer {
		t.Fatalf("expected server role, got %q", got)
	}
	if got := DetectUnitRole("  ExecStart=tungo c\n"); got != UnitRoleClient {
		t.Fatalf("expected client role with leading spaces, got %q", got)
	}
	if got := DetectUnitRole("\tExecStart=tungo s\n"); got != UnitRoleServer {
		t.Fatalf("expected server role with leading tab, got %q", got)
	}
	if got := DetectUnitRole("ExecStart=/usr/bin/other\n"); got != UnitRoleUnknown {
		t.Fatalf("expected unknown role, got %q", got)
	}
	if got := DetectUnitRole("NoExecStart=1\n"); got != UnitRoleUnknown {
		t.Fatalf("expected unknown role when ExecStart missing, got %q", got)
	}
}

func TestDetectUnitRoleFromExecStart(t *testing.T) {
	cases := []struct {
		execStart string
		want      UnitRole
	}{
		{"", UnitRoleUnknown},
		{"unknown", UnitRoleUnknown},
		{"/usr/local/bin/tungo c", UnitRoleClient},
		{"/usr/local/bin/tungo s", UnitRoleServer},
		{"{ path=/usr/local/bin/tungo ; argv[]=/usr/local/bin/tungo ; argv[]=s ; }", UnitRoleServer},
		{"{ path=/usr/local/bin/tungo ; argv[]=/usr/local/bin/tungo ; argv[]=c ; }", UnitRoleClient},
		{"/usr/bin/env ABC=1 /usr/local/bin/tungo c --flag", UnitRoleClient},
		{"/usr/bin/env tungo s --foreground", UnitRoleServer},
		{"/usr/local/bin/tungo -c config.yaml", UnitRoleUnknown},
		{"/usr/local/bin/tungo --profile=s", UnitRoleUnknown},
		{"/usr/bin/other", UnitRoleUnknown},
	}
	for _, tc := range cases {
		if got := DetectUnitRoleFromExecStart(tc.execStart); got != tc.want {
			t.Fatalf("execStart=%q: got %q want %q", tc.execStart, got, tc.want)
		}
	}
}
