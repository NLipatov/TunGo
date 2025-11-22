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
	wantTag := "v1.2.3-test"
	Tag = wantTag // imitate ldflags injection

	got := capture(func() { NewRunner().Run(context.Background()) })

	want := app.Name + " " + wantTag
	if !strings.Contains(got, want) {
		t.Fatalf("stdout = %q, want substring %q", got, want)
	}
}
