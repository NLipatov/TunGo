package commandline

import (
	"strings"
	"testing"
	"tungo/runtime"
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
