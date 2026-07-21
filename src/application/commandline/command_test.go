package commandline

import (
	"strings"
	"testing"
	"tungo/application/runtime"
)

func TestParseCommandOK(t *testing.T) {
	cases := []struct {
		in   []string
		want Command
	}{
		{[]string{"c"}, Command{Kind: CommandRuntime, RuntimeMode: runtime.ModeClient, RequiresElevation: true}},
		{[]string{"s"}, Command{Kind: CommandRuntime, RuntimeMode: runtime.ModeServer, RequiresElevation: true}},
		{[]string{"  c  "}, Command{Kind: CommandRuntime, RuntimeMode: runtime.ModeClient, RequiresElevation: true}},
		{[]string{"s", "gen"}, Command{Kind: CommandServerConfigGenerate, RequiresElevation: true}},
		{[]string{" version "}, Command{Kind: CommandVersion}},
	}

	for _, c := range cases {
		got, err := ParseCommand(c.in)
		if err != nil || got != c.want {
			t.Fatalf("args=%v want=%+v got=%+v err=%v", c.in, c.want, got, err)
		}
	}
}

func TestParseCommandErrors(t *testing.T) {
	got, err := ParseCommand(nil)
	if err == nil || got.Kind != CommandUnknown {
		t.Fatalf("expected unknown command error for no args, got %+v err=%v", got, err)
	}

	got, err = ParseCommand([]string{"???", "abc"})
	if err == nil || got.Kind != CommandUnknown {
		t.Fatalf("expected unknown command error for invalid args, got %+v err=%v", got, err)
	}
}

func TestRuntimeModeArgs(t *testing.T) {
	got, err := RuntimeModeArgs(runtime.ModeServer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(got, " ") != "s" {
		t.Fatalf("expected server args, got %v", got)
	}

	got[0] = "mutated"
	got, err = RuntimeModeArgs(runtime.ModeServer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(got, " ") != "s" {
		t.Fatalf("expected returned args to be isolated, got %v", got)
	}

	got, err = RuntimeModeArgs(runtime.ModeClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(got, " ") != "c" {
		t.Fatalf("expected client args, got %v", got)
	}

	if _, err := RuntimeModeArgs(0); err == nil {
		t.Fatal("expected error for invalid runtime mode")
	}
}

func TestCommandUsage(t *testing.T) {
	got := CommandUsage("tungo")
	if !strings.Contains(got, "Usage: tungo <command>") ||
		!strings.Contains(got, "s  - Start server runtime") ||
		!strings.Contains(got, "c  - Start client runtime") ||
		!strings.Contains(got, "s gen  - Generate server configuration") ||
		!strings.Contains(got, "version  - Show version") {
		t.Fatalf("unexpected usage: %q", got)
	}
}
