package bubble_tea

import (
	"errors"
	"os"
	"testing"
	"time"
)

type fakeFileInfo struct {
	mode os.FileMode
}

func (f fakeFileInfo) Name() string       { return "fake" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }

func withRuntimeEnvHooks(
	t *testing.T,
	term func() string,
	stdin func() (os.FileInfo, error),
	stdout func() (os.FileInfo, error),
) {
	t.Helper()
	prevTerm := getTermEnv
	prevStdin := stdinStat
	prevStdout := stdoutStat
	getTermEnv = term
	stdinStat = stdin
	stdoutStat = stdout
	t.Cleanup(func() {
		getTermEnv = prevTerm
		stdinStat = prevStdin
		stdoutStat = prevStdout
	})
}

func TestIsInteractiveTerminal_FalseForEmptyAndDumbTERM(t *testing.T) {
	withRuntimeEnvHooks(
		t,
		func() string { return "" },
		func() (os.FileInfo, error) { return fakeFileInfo{mode: os.ModeCharDevice}, nil },
		func() (os.FileInfo, error) { return fakeFileInfo{mode: os.ModeCharDevice}, nil },
	)
	if IsInteractiveTerminal() {
		t.Fatal("expected non-interactive terminal when TERM is empty")
	}

	withRuntimeEnvHooks(
		t,
		func() string { return "dumb" },
		func() (os.FileInfo, error) { return fakeFileInfo{mode: os.ModeCharDevice}, nil },
		func() (os.FileInfo, error) { return fakeFileInfo{mode: os.ModeCharDevice}, nil },
	)
	if IsInteractiveTerminal() {
		t.Fatal("expected non-interactive terminal when TERM is dumb")
	}
}

func TestIsInteractiveTerminal_FalseWhenStdinStatFails(t *testing.T) {
	withRuntimeEnvHooks(
		t,
		func() string { return "xterm-256color" },
		func() (os.FileInfo, error) { return nil, errors.New("stdin failed") },
		func() (os.FileInfo, error) { return fakeFileInfo{mode: os.ModeCharDevice}, nil },
	)
	if IsInteractiveTerminal() {
		t.Fatal("expected non-interactive terminal when stdin stat fails")
	}
}

func TestIsInteractiveTerminal_FalseWhenStdoutStatFails(t *testing.T) {
	withRuntimeEnvHooks(
		t,
		func() string { return "xterm-256color" },
		func() (os.FileInfo, error) { return fakeFileInfo{mode: os.ModeCharDevice}, nil },
		func() (os.FileInfo, error) { return nil, errors.New("stdout failed") },
	)
	if IsInteractiveTerminal() {
		t.Fatal("expected non-interactive terminal when stdout stat fails")
	}
}

func TestIsInteractiveTerminal_FalseWhenNotCharDevice(t *testing.T) {
	withRuntimeEnvHooks(
		t,
		func() string { return "xterm-256color" },
		func() (os.FileInfo, error) { return fakeFileInfo{mode: 0}, nil },
		func() (os.FileInfo, error) { return fakeFileInfo{mode: os.ModeCharDevice}, nil },
	)
	if IsInteractiveTerminal() {
		t.Fatal("expected non-interactive terminal when stdin is not a char device")
	}
}

func TestIsInteractiveTerminal_True(t *testing.T) {
	withRuntimeEnvHooks(
		t,
		func() string { return "xterm-256color" },
		func() (os.FileInfo, error) { return fakeFileInfo{mode: os.ModeCharDevice}, nil },
		func() (os.FileInfo, error) { return fakeFileInfo{mode: os.ModeCharDevice}, nil },
	)
	if !IsInteractiveTerminal() {
		t.Fatal("expected interactive terminal")
	}
}

func TestRuntimeEnvironment_DefaultHooks_AreCallable(t *testing.T) {
	_ = getTermEnv()
	_, _ = stdinStat()
	_, _ = stdoutStat()
}
