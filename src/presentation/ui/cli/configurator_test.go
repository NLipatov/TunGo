package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"tungo/domain/command"
)

/*
run sets up fake os.Args, redirects stdout to a pipe, calls Configure,
restores globals, and returns captured output, resulting mode, and error.
*/
func run(args []string) (out string, got command.Command, err error) {
	origArgs, origStd := os.Args, os.Stdout
	defer func() { os.Args, os.Stdout = origArgs, origStd }()

	os.Args = append([]string{"app"}, args...)

	// redirect Stdout to a pipe
	r, w, _ := os.Pipe()
	os.Stdout = w

	got, err = NewConfigurator().Configure(context.Background())

	_ = w.Close() // close writer so reader receives EOF
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r) // read everything printed to stdout
	out = buf.String()
	return
}

func TestConfigureOK(t *testing.T) {
	cases := []struct {
		in   []string
		want command.Command
	}{
		{[]string{"c"}, command.StartClient},
		{[]string{"s"}, command.StartServer},
		{[]string{"s", "gen"}, command.GenerateClientConfig},
		{[]string{"version"}, command.ShowVersion},
	}

	for _, c := range cases {
		out, got, err := run(c.in)
		if err != nil || got != c.want || out != "" {
			t.Fatalf("args=%v want=%v got=%v err=%v stdout=%q",
				c.in, c.want, got, err, out)
		}
	}
}

func TestHelpRequest(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"help"}} {
		if !IsHelpRequest(args) {
			t.Fatalf("expected %v to be a help request", args)
		}
	}

	for _, args := range [][]string{nil, {"c"}, {"help", "extra"}} {
		if IsHelpRequest(args) {
			t.Fatalf("expected %v not to be a help request", args)
		}
	}
}

func TestConfigureErrors(t *testing.T) {
	// no arguments
	out, got, err := run(nil)
	if err == nil || got != command.Unknown || !strings.Contains(out, "Usage:") {
		t.Fatalf("expected usage banner for no args")
	}

	// unknown arguments
	out, got, err = run([]string{"???", "abc"})
	if err == nil || got != command.Unknown || !strings.Contains(out, "Usage:") {
		t.Fatalf("expected usage banner for invalid args")
	}
}

func TestUsageListsSupportedCommands(t *testing.T) {
	out, _, _ := run(nil)
	for _, want := range []string{
		"Usage:",
		"  tungo <command>",
		"Commands:",
		"  s        Start a server",
		"  c        Start a client",
		"  s gen    Generate client configuration",
		"  version  Show version",
		"  help     Show help",
		"Options:",
		"  -h, --help  Show help",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected usage to contain %q, got %q", want, out)
		}
	}
}
