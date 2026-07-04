package version

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"tungo/domain/app"
)

func capture(f func()) string {
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	os.Stdout = orig
	return buf.String()
}

func TestRunner_Run_PrintsVersion(t *testing.T) {
	prevTag := Tag
	t.Cleanup(func() { Tag = prevTag })

	wantTag := "v1.2.3-test"
	Tag = wantTag // imitate ldflags injection

	got := capture(func() { NewRunner().Run(context.Background()) })

	want := app.Name + " " + wantTag
	if !strings.Contains(got, want) {
		t.Fatalf("stdout = %q, want substring %q", got, want)
	}
}

func TestRunner_Run_PrintsDevBuildWhenTagUnset(t *testing.T) {
	prevTag := Tag
	t.Cleanup(func() { Tag = prevTag })

	Tag = "dev-build"
	got := capture(func() { NewRunner().Run(context.Background()) })
	want := app.Name + " dev-build"
	if !strings.Contains(got, want) {
		t.Fatalf("stdout = %q, want substring %q", got, want)
	}
}

func TestCurrent(t *testing.T) {
	prevTag := Tag
	t.Cleanup(func() { Tag = prevTag })

	Tag = " v0.3.0 "
	if got := Current(); got != "v0.3.0" {
		t.Fatalf("expected trimmed tag, got %q", got)
	}

	Tag = ""
	if got := Current(); got != "" {
		t.Fatalf("expected empty value for empty tag, got %q", got)
	}
}
