package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"tungo/runtime"
)

/*
run sets up fake os.Args, redirects stdout to a pipe, calls Configure,
restores globals, and returns captured output, resulting mode, and error.
*/
func run(args []string) (out string, got runtime.Mode, err error) {
	origArgs, origStd := os.Args, os.Stdout
	defer func() { os.Args, os.Stdout = origArgs, origStd }()

	os.Args = append([]string{"app"}, args...)

	// redirect Stdout to a pipe
	r, w, _ := os.Pipe()
	os.Stdout = w

	got, err = Configure(context.Background())

	_ = w.Close() // close writer so reader receives EOF
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r) // read everything printed to stdout
	out = buf.String()
	return
}

func TestConfigureOK(t *testing.T) {
	cases := []struct {
		in   []string
		want runtime.Mode
	}{
		{[]string{"c"}, runtime.ModeClient},
		{[]string{"s"}, runtime.ModeServer},
	}

	for _, c := range cases {
		out, got, err := run(c.in)
		if err != nil || got != c.want || out != "" {
			t.Fatalf("args=%v want=%v got=%v err=%v stdout=%q",
				c.in, c.want, got, err, out)
		}
	}
}

func TestConfigureErrors(t *testing.T) {
	// no arguments
	out, got, err := run(nil)
	if err == nil || got != 0 || !strings.Contains(out, "Usage:") {
		t.Fatalf("expected usage banner for no args")
	}

	// unknown arguments
	out, got, err = run([]string{"???", "abc"})
	if err == nil || got != 0 || !strings.Contains(out, "Usage:") {
		t.Fatalf("expected usage banner for invalid args")
	}
}
