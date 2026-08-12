package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"tungo/internal/elevation"
	"tungo/internal/product"
)

const runMainVersionEnv = "TUNGO_TEST_RUN_MAIN_VERSION"

func setCommandLine(t *testing.T, args ...string) {
	t.Helper()
	previous := os.Args
	os.Args = append([]string{"tungo"}, args...)
	t.Cleanup(func() { os.Args = previous })
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = previous
		_ = reader.Close()
		_ = writer.Close()
	})

	fn()
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	os.Stdout = previous
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(output)
}

func TestRunCLI_InvalidArguments(t *testing.T) {
	setCommandLine(t, "invalid")
	var runErr error
	output := captureStdout(t, func() {
		runErr = runCLI(context.Background())
	})

	if runErr == nil || !strings.Contains(runErr.Error(), "configuration error") {
		t.Fatalf("runCLI() error = %v", runErr)
	}
	if !strings.Contains(output, "Usage: tungo <command>") {
		t.Fatalf("usage output = %q", output)
	}
}

func TestMain_Version(t *testing.T) {
	if os.Getenv(runMainVersionEnv) == "1" {
		os.Args = []string{"tungo", "version"}
		main()
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestMain_Version$")
	command.Env = append(os.Environ(), runMainVersionEnv+"=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("main process error = %v; output: %s", err, output)
	}
	if strings.TrimSpace(string(output)) == "" {
		t.Fatal("expected version output")
	}
}

func TestRunCLI_Version(t *testing.T) {
	setCommandLine(t, "version")
	previousVersion := product.Version
	product.Version = "v-test"
	t.Cleanup(func() { product.Version = previousVersion })

	var runErr error
	output := captureStdout(t, func() {
		runErr = runCLI(context.Background())
	})

	if runErr != nil {
		t.Fatalf("runCLI() error = %v", runErr)
	}
	want := product.Name + " v-test\n"
	if output != want {
		t.Fatalf("version output = %q, want %q", output, want)
	}
}

func TestRequireElevationMatchesPlatformState(t *testing.T) {
	err := requireElevation()
	if elevation.IsElevated() {
		if err != nil {
			t.Fatalf("requireElevation() error = %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), "admin privileges") {
		t.Fatalf("requireElevation() error = %v", err)
	}
}
