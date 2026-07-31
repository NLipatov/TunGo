package systemd

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"tungo/application/runtime"
)

func TestSetupInactiveUnitInstallsWithoutRestart(t *testing.T) {
	var unitBody string
	withAvailableSetupHooks(t, func(_ string, body []byte, _ os.FileMode) error {
		unitBody = string(body)
		return nil
	})
	commander := &mockCommander{combinedOutput: []byte("inactive\n")}
	installer := NewUnitInstaller(commander)

	path, err := installer.Setup(runtime.ModeServer)
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if path != systemdUnitPath {
		t.Fatalf("Setup() path = %q, want %q", path, systemdUnitPath)
	}
	if !strings.Contains(unitBody, " s") {
		t.Fatalf("server unit body does not contain server mode: %q", unitBody)
	}
	assertSystemctlCalls(t, commander.runCalls, "daemon-reload", "enable")
}

func TestSetupActiveUnitStopsInstallsAndRestarts(t *testing.T) {
	withAvailableSetupHooks(t, func(string, []byte, os.FileMode) error { return nil })
	commander := &mockCommander{combinedOutput: []byte("active\n")}
	installer := NewUnitInstaller(commander)

	path, err := installer.Setup(runtime.ModeClient)
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if path != systemdUnitPath {
		t.Fatalf("Setup() path = %q, want %q", path, systemdUnitPath)
	}
	assertSystemctlCalls(t, commander.runCalls, "stop", "daemon-reload", "enable", "start")
}

func TestSetupRejectsInvalidModeBeforeSystemdCalls(t *testing.T) {
	withAvailableSetupHooks(t, func(string, []byte, os.FileMode) error { return nil })
	commander := &mockCommander{}
	installer := NewUnitInstaller(commander)

	_, err := installer.Setup(0)
	if err == nil || !strings.Contains(err.Error(), "invalid daemon mode") {
		t.Fatalf("Setup() error = %v, want invalid daemon mode", err)
	}
	if len(commander.combinedOutputCalls) != 0 || len(commander.runCalls) != 0 {
		t.Fatalf(
			"systemd calls = combined:%v run:%v, want none",
			commander.combinedOutputCalls,
			commander.runCalls,
		)
	}
}

func TestSetupReturnsStatusCheckError(t *testing.T) {
	wantErr := errors.New("status failed")
	withAvailableSetupHooks(t, func(string, []byte, os.FileMode) error { return nil })
	commander := &mockCommander{combinedOutputErr: wantErr}
	installer := NewUnitInstaller(commander)

	_, err := installer.Setup(runtime.ModeClient)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Setup() error = %v, want wrapping %v", err, wantErr)
	}
}

func TestSetupReturnsInstallError(t *testing.T) {
	wantErr := errors.New("write failed")
	withAvailableSetupHooks(t, func(string, []byte, os.FileMode) error { return wantErr })
	commander := &mockCommander{combinedOutput: []byte("inactive\n")}
	installer := NewUnitInstaller(commander)

	_, err := installer.Setup(runtime.ModeServer)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Setup() error = %v, want wrapping %v", err, wantErr)
	}
}

func TestSetupRestartsActiveUnitAfterInstallError(t *testing.T) {
	wantErr := errors.New("write failed")
	withAvailableSetupHooks(t, func(string, []byte, os.FileMode) error { return wantErr })
	commander := &mockCommander{combinedOutput: []byte("active\n")}
	installer := NewUnitInstaller(commander)

	_, err := installer.Setup(runtime.ModeServer)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Setup() error = %v, want wrapping %v", err, wantErr)
	}
	assertSystemctlCalls(t, commander.runCalls, "stop", "start")
}

func TestSetupPreservesInstallErrorWhenRecoveryRestartFails(t *testing.T) {
	installErr := errors.New("write failed")
	restartErr := errors.New("restart failed")
	withAvailableSetupHooks(t, func(string, []byte, os.FileMode) error { return installErr })
	commander := &mockCommander{
		combinedOutput: []byte("active\n"),
		runErrByArg:    map[string]error{"start": restartErr},
	}
	installer := NewUnitInstaller(commander)

	_, err := installer.Setup(runtime.ModeServer)
	if !errors.Is(err, installErr) {
		t.Fatalf("Setup() error = %v, want wrapping %v", err, installErr)
	}
	if !strings.Contains(err.Error(), restartErr.Error()) {
		t.Fatalf("Setup() error = %v, want recovery error %v", err, restartErr)
	}
	assertSystemctlCalls(t, commander.runCalls, "stop", "start")
}

func TestSetupStopsWhenActiveUnitCannotBeStopped(t *testing.T) {
	wantErr := errors.New("stop failed")
	withAvailableSetupHooks(t, func(string, []byte, os.FileMode) error { return nil })
	commander := &mockCommander{
		combinedOutput: []byte("active\n"),
		runErrByArg:    map[string]error{"stop": wantErr},
	}
	installer := NewUnitInstaller(commander)

	_, err := installer.Setup(runtime.ModeClient)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Setup() error = %v, want wrapping %v", err, wantErr)
	}
	assertSystemctlCalls(t, commander.runCalls, "stop")
}

func TestSetupReturnsRestartError(t *testing.T) {
	wantErr := errors.New("start failed")
	withAvailableSetupHooks(t, func(string, []byte, os.FileMode) error { return nil })
	commander := &mockCommander{
		combinedOutput: []byte("active\n"),
		runErrByArg:    map[string]error{"start": wantErr},
	}
	installer := NewUnitInstaller(commander)

	_, err := installer.Setup(runtime.ModeServer)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Setup() error = %v, want wrapping %v", err, wantErr)
	}
	assertSystemctlCalls(t, commander.runCalls, "stop", "daemon-reload", "enable", "start")
}

func withAvailableSetupHooks(
	t *testing.T,
	write func(string, []byte, os.FileMode) error,
) {
	t.Helper()
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) { return "/bin/" + name, nil },
		write,
	)
}

func assertSystemctlCalls(t *testing.T, calls [][2]string, want ...string) {
	t.Helper()
	got := make([]string, 0, len(calls))
	for _, call := range calls {
		if call[0] == "systemctl" {
			got = append(got, call[1])
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("systemctl calls = %v, want %v", got, want)
	}
}
