package systemd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

type mockCommander struct {
	runCalls            [][2]string
	runErr              error
	runErrByArg         map[string]error
	combinedOutputCalls [][2]string
	combinedOutput      []byte
	combinedOutputErr   error
	combinedOutputByArg map[string]mockCombinedOutputResult
}

type mockCombinedOutputResult struct {
	output []byte
	err    error
}

type mockFileInfo struct {
	name string
	mode os.FileMode
	sys  interface{}
}

func (m mockFileInfo) Name() string       { return m.name }
func (m mockFileInfo) Size() int64        { return 0 }
func (m mockFileInfo) Mode() os.FileMode  { return m.mode }
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return m.mode.IsDir() }
func (m mockFileInfo) Sys() interface{}   { return m.sys }

type mockStatWithUID struct {
	Uid uint64
}

func statWithUID(uid uint64) interface{} {
	return &mockStatWithUID{Uid: uid}
}

func (m *mockCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	arg0 := ""
	if len(args) > 0 {
		arg0 = args[0]
	}
	m.combinedOutputCalls = append(m.combinedOutputCalls, [2]string{name, arg0})
	if result, ok := m.combinedOutputByArg[arg0]; ok {
		return result.output, result.err
	}
	return m.combinedOutput, m.combinedOutputErr
}

func (m *mockCommander) Output(name string, args ...string) ([]byte, error) {
	return m.CombinedOutput(name, args...)
}

func (m *mockCommander) Run(name string, args ...string) error {
	arg0 := ""
	if len(args) > 0 {
		arg0 = args[0]
	}
	m.runCalls = append(m.runCalls, [2]string{name, arg0})
	if err, ok := m.runErrByArg[arg0]; ok {
		return err
	}
	return m.runErr
}

func withSystemdHooks(
	t *testing.T,
	stat func(string) (os.FileInfo, error),
	look func(string) (string, error),
	write func(string, []byte, os.FileMode) error,
	read ...func(string) ([]byte, error),
) {
	t.Helper()
	prevStat := statPath
	prevLstat := lstatPath
	prevLook := lookPath
	prevWrite := writeFilePath
	prevRead := readFilePath
	prevRemove := removePath
	prevGeteuid := geteuid
	readHook := func(string) ([]byte, error) { return []byte(""), nil }
	if len(read) > 0 && read[0] != nil {
		readHook = read[0]
	}
	statPath = func(path string) (os.FileInfo, error) {
		info, err := stat(path)
		if err != nil {
			return nil, err
		}
		if info != nil {
			return info, nil
		}
		switch path {
		case systemdRuntimeDir:
			return mockFileInfo{name: "system", mode: os.ModeDir | 0o755, sys: statWithUID(0)}, nil
		case tungoBinaryPath:
			return mockFileInfo{name: "tungo", mode: 0o755, sys: statWithUID(0)}, nil
		case systemdUnitPath:
			return mockFileInfo{name: "tungo.service", mode: 0o644, sys: statWithUID(0)}, nil
		default:
			return mockFileInfo{name: "file", mode: 0o644, sys: statWithUID(0)}, nil
		}
	}
	lstatPath = statPath
	lookPath = look
	writeFilePath = write
	readFilePath = readHook
	removePath = func(string) error { return nil }
	geteuid = func() int { return 0 }
	t.Cleanup(func() {
		statPath = prevStat
		lstatPath = prevLstat
		lookPath = prevLook
		writeFilePath = prevWrite
		readFilePath = prevRead
		removePath = prevRemove
		geteuid = prevGeteuid
	})
}

func TestSupported_TrueWhenRuntimeDirAndSystemctlExist(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	installer := NewUnitInstaller(&mockCommander{})
	if !installer.Supported() {
		t.Fatal("expected Supported()=true")
	}
}

func TestSupported_FalseWhenRuntimeDirMissing(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	installer := NewUnitInstaller(&mockCommander{})
	if installer.Supported() {
		t.Fatal("expected Supported()=false when /run/systemd/system is missing")
	}
}

func TestSupported_FalseWhenSystemctlMissing(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "", exec.ErrNotFound
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	installer := NewUnitInstaller(&mockCommander{})
	if installer.Supported() {
		t.Fatal("expected Supported()=false when systemctl is missing")
	}
}

func TestNewUnitInstaller_NilCommander_UsesDefault(t *testing.T) {
	installer := NewUnitInstaller(nil)
	concrete, ok := installer.(*UnitInstaller)
	if !ok {
		t.Fatalf("expected *UnitInstaller, got %T", installer)
	}
	if concrete.commander == nil {
		t.Fatal("expected default commander when nil commander is provided")
	}
}

func TestInstallServerUnit_WritesServerModeAndEnablesService(t *testing.T) {
	var gotPath string
	var gotContent string
	var gotPerm os.FileMode
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(path string, data []byte, perm os.FileMode) error {
			gotPath = path
			gotContent = string(data)
			gotPerm = perm
			return nil
		},
	)
	cmd := &mockCommander{}
	installer := NewUnitInstaller(cmd)

	path, err := installer.InstallServerUnit()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != systemdUnitPath {
		t.Fatalf("path: got %q want %q", path, systemdUnitPath)
	}
	if gotPath != systemdUnitPath {
		t.Fatalf("write path: got %q want %q", gotPath, systemdUnitPath)
	}
	if gotPerm != 0644 {
		t.Fatalf("write perm: got %v want 0644", gotPerm)
	}
	if !strings.Contains(gotContent, "ExecStart=/usr/local/bin/tungo s") {
		t.Fatalf("expected server unit content, got %q", gotContent)
	}
	if len(cmd.runCalls) != 2 {
		t.Fatalf("expected 2 systemctl calls, got %d", len(cmd.runCalls))
	}
	if cmd.runCalls[0] != [2]string{"systemctl", "daemon-reload"} {
		t.Fatalf("unexpected first command: %v", cmd.runCalls[0])
	}
	if cmd.runCalls[1] != [2]string{"systemctl", "enable"} {
		t.Fatalf("unexpected second command: %v", cmd.runCalls[1])
	}
}

func TestInstallClientUnit_WritesClientMode(t *testing.T) {
	var gotContent string
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(_ string, data []byte, _ os.FileMode) error {
			gotContent = string(data)
			return nil
		},
	)
	cmd := &mockCommander{}
	installer := NewUnitInstaller(cmd)

	_, err := installer.InstallClientUnit()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotContent, "ExecStart=/usr/local/bin/tungo c") {
		t.Fatalf("expected client unit content, got %q", gotContent)
	}
}

func TestInstallUnit_FailsWhenTungoBinaryMissing(t *testing.T) {
	withSystemdHooks(
		t,
		func(path string) (os.FileInfo, error) {
			if path == tungoBinaryPath {
				return nil, os.ErrNotExist
			}
			return nil, nil
		},
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{}
	installer := NewUnitInstaller(cmd)

	_, err := installer.InstallClientUnit()
	if err == nil || !strings.Contains(err.Error(), "tungo executable is not installed at /usr/local/bin/tungo") {
		t.Fatalf("expected missing /usr/local/bin/tungo error, got %v", err)
	}
	if len(cmd.runCalls) != 0 {
		t.Fatalf("expected no systemctl calls, got %v", cmd.runCalls)
	}
}

func TestInstallUnit_FailsWhenTungoBinaryNotExecutable(t *testing.T) {
	withSystemdHooks(
		t,
		func(path string) (os.FileInfo, error) {
			if path == tungoBinaryPath {
				return mockFileInfo{name: "tungo", mode: 0o644, sys: statWithUID(0)}, nil
			}
			return nil, nil
		},
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{}
	installer := NewUnitInstaller(cmd)

	_, err := installer.InstallClientUnit()
	if err == nil || !strings.Contains(err.Error(), "is not executable") {
		t.Fatalf("expected not executable error, got %v", err)
	}
}

func TestInstallUnit_FailsWhenTungoBinaryIsSymlink(t *testing.T) {
	withSystemdHooks(
		t,
		func(path string) (os.FileInfo, error) {
			if path == tungoBinaryPath {
				return mockFileInfo{name: "tungo", mode: os.ModeSymlink | 0o777, sys: statWithUID(0)}, nil
			}
			return nil, nil
		},
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{}
	installer := NewUnitInstaller(cmd)

	_, err := installer.InstallClientUnit()
	if err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

func TestInstallUnit_FailsWhenTungoBinaryOwnedByNonRoot(t *testing.T) {
	withSystemdHooks(
		t,
		func(path string) (os.FileInfo, error) {
			if path == tungoBinaryPath {
				return mockFileInfo{name: "tungo", mode: 0o755, sys: statWithUID(1000)}, nil
			}
			return nil, nil
		},
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{}
	installer := NewUnitInstaller(cmd)

	_, err := installer.InstallClientUnit()
	if err == nil || !strings.Contains(err.Error(), "must be owned by root") {
		t.Fatalf("expected non-root owner error, got %v", err)
	}
}

func TestInstallUnit_FailsWhenTungoBinaryGroupWritable(t *testing.T) {
	withSystemdHooks(
		t,
		func(path string) (os.FileInfo, error) {
			if path == tungoBinaryPath {
				return mockFileInfo{name: "tungo", mode: 0o775, sys: statWithUID(0)}, nil
			}
			return nil, nil
		},
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{}
	installer := NewUnitInstaller(cmd)

	_, err := installer.InstallClientUnit()
	if err == nil || !strings.Contains(err.Error(), "must not be writable by group or others") {
		t.Fatalf("expected unsafe mode error, got %v", err)
	}
}

func TestInstallUnit_FailsWhenSystemctlCommandFails(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{runErr: errors.New("boom")}
	installer := NewUnitInstaller(cmd)

	_, err := installer.InstallServerUnit()
	if err == nil || !strings.Contains(err.Error(), "daemon-reload") {
		t.Fatalf("expected daemon-reload error, got %v", err)
	}
}

func TestInstallUnit_DaemonReloadFailure_RollsBackWrittenUnit(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	removedPath := ""
	removePath = func(path string) error {
		removedPath = path
		return nil
	}
	cmd := &mockCommander{
		runErrByArg: map[string]error{
			"daemon-reload": errors.New("reload failed"),
		},
	}
	installer := NewUnitInstaller(cmd)

	_, err := installer.InstallClientUnit()
	if err == nil || !strings.Contains(err.Error(), "daemon-reload") {
		t.Fatalf("expected daemon-reload error, got %v", err)
	}
	if removedPath != systemdUnitPath {
		t.Fatalf("expected rollback remove path %q, got %q", systemdUnitPath, removedPath)
	}
}

func TestInstallUnit_FailsWhenSystemdUnsupported(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		func(string) (string, error) { return "", exec.ErrNotFound },
		func(string, []byte, os.FileMode) error { return nil },
	)
	installer := NewUnitInstaller(&mockCommander{})
	_, err := installer.InstallClientUnit()
	if err == nil || !strings.Contains(err.Error(), "systemd is not supported") {
		t.Fatalf("expected unsupported systemd error, got %v", err)
	}
}

func TestInstallUnit_FailsWhenWriteFails(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return errors.New("write failed") },
	)
	installer := NewUnitInstaller(&mockCommander{})
	_, err := installer.InstallClientUnit()
	if err == nil || !strings.Contains(err.Error(), "failed to write") {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestInstallUnit_FailsWhenEnableFails(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{
		runErrByArg: map[string]error{
			"enable": errors.New("enable failed"),
		},
	}
	installer := NewUnitInstaller(cmd)
	_, err := installer.InstallClientUnit()
	if err == nil || !strings.Contains(err.Error(), "systemctl enable") {
		t.Fatalf("expected enable error, got %v", err)
	}
}

func TestInstallUnit_EnableFailure_RollsBackWrittenUnit(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	removedPath := ""
	removePath = func(path string) error {
		removedPath = path
		return nil
	}
	cmd := &mockCommander{
		runErrByArg: map[string]error{
			"enable": errors.New("enable failed"),
		},
	}
	installer := NewUnitInstaller(cmd)

	_, err := installer.InstallClientUnit()
	if err == nil || !strings.Contains(err.Error(), "systemctl enable") {
		t.Fatalf("expected enable error, got %v", err)
	}
	if removedPath != systemdUnitPath {
		t.Fatalf("expected rollback remove path %q, got %q", systemdUnitPath, removedPath)
	}
}

func TestIsUnitActive_ReturnsTrueWhenServiceIsActive(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{combinedOutput: []byte("active\n")}
	installer := NewUnitInstaller(cmd)

	active, err := installer.IsUnitActive()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Fatal("expected active=true")
	}
	if len(cmd.combinedOutputCalls) != 1 || cmd.combinedOutputCalls[0] != [2]string{"systemctl", "is-active"} {
		t.Fatalf("unexpected combined output calls: %v", cmd.combinedOutputCalls)
	}
}

func TestIsUnitActive_ReturnsTrueWhenServiceIsActivating(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{combinedOutput: []byte("activating\n")}
	installer := NewUnitInstaller(cmd)

	active, err := installer.IsUnitActive()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Fatal("expected active=true when service is activating")
	}
}

func TestIsUnitActive_ReturnsTrueWhenServiceIsDeactivating(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{combinedOutput: []byte("deactivating\n")}
	installer := NewUnitInstaller(cmd)

	active, err := installer.IsUnitActive()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Fatal("expected active=true when service is deactivating")
	}
}

func TestIsUnitActive_ReturnsTrueWhenServiceIsReloading(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{combinedOutput: []byte("reloading\n")}
	installer := NewUnitInstaller(cmd)

	active, err := installer.IsUnitActive()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Fatal("expected active=true when service is reloading")
	}
}

func TestIsUnitActive_ReturnsFalseWhenServiceIsInactive(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{combinedOutputErr: commandExitError(t, 3)}
	installer := NewUnitInstaller(cmd)

	active, err := installer.IsUnitActive()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Fatal("expected active=false when service is inactive")
	}
}

func TestIsUnitActive_ReturnsFalseWhenServiceIsMissing(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{combinedOutputErr: commandExitError(t, 4)}
	installer := NewUnitInstaller(cmd)

	active, err := installer.IsUnitActive()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Fatal("expected active=false when unit is missing")
	}
}

func TestIsUnitActive_FailsOnUnexpectedError(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{combinedOutputErr: errors.New("boom")}
	installer := NewUnitInstaller(cmd)

	_, err := installer.IsUnitActive()
	if err == nil || !strings.Contains(err.Error(), "is-active") {
		t.Fatalf("expected is-active error, got %v", err)
	}
}

func TestIsUnitActive_FailsWhenSystemdUnsupported(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		func(string) (string, error) { return "", exec.ErrNotFound },
		func(string, []byte, os.FileMode) error { return nil },
	)
	installer := NewUnitInstaller(&mockCommander{})
	_, err := installer.IsUnitActive()
	if err == nil || !strings.Contains(err.Error(), "systemd is not supported") {
		t.Fatalf("expected unsupported error, got %v", err)
	}
}

func TestStopUnit_RunsSystemctlStop(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{}
	installer := NewUnitInstaller(cmd)

	if err := installer.StopUnit(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.runCalls) != 1 || cmd.runCalls[0] != [2]string{"systemctl", "stop"} {
		t.Fatalf("unexpected run calls: %v", cmd.runCalls)
	}
}

func TestStopUnit_FailsOnCommandError(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{runErr: errors.New("boom")}
	installer := NewUnitInstaller(cmd)

	err := installer.StopUnit()
	if err == nil || !strings.Contains(err.Error(), "systemctl stop") {
		t.Fatalf("expected stop error, got %v", err)
	}
}

func TestStartUnit_RunsSystemctlStart(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{}
	installer := NewUnitInstaller(cmd)

	if err := installer.StartUnit(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.runCalls) != 1 || cmd.runCalls[0] != [2]string{"systemctl", "start"} {
		t.Fatalf("unexpected run calls: %v", cmd.runCalls)
	}
}

func TestStartUnit_FailsOnCommandError(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{runErr: errors.New("boom")}
	installer := NewUnitInstaller(cmd)
	err := installer.StartUnit()
	if err == nil || !strings.Contains(err.Error(), "systemctl start") {
		t.Fatalf("expected start error, got %v", err)
	}
}

func TestEnableUnit_RunsSystemctlEnable(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{}
	installer := NewUnitInstaller(cmd)

	if err := installer.EnableUnit(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.runCalls) != 1 || cmd.runCalls[0] != [2]string{"systemctl", "enable"} {
		t.Fatalf("unexpected run calls: %v", cmd.runCalls)
	}
}

func TestEnableUnit_FailsOnCommandError(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{runErr: errors.New("boom")}
	installer := NewUnitInstaller(cmd)
	err := installer.EnableUnit()
	if err == nil || !strings.Contains(err.Error(), "systemctl enable") {
		t.Fatalf("expected enable error, got %v", err)
	}
}

func TestDisableUnit_RunsSystemctlDisable(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{}
	installer := NewUnitInstaller(cmd)

	if err := installer.DisableUnit(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.runCalls) != 1 || cmd.runCalls[0] != [2]string{"systemctl", "disable"} {
		t.Fatalf("unexpected run calls: %v", cmd.runCalls)
	}
}

func TestDisableUnit_FailsOnCommandError(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{runErr: errors.New("boom")}
	installer := NewUnitInstaller(cmd)
	err := installer.DisableUnit()
	if err == nil || !strings.Contains(err.Error(), "systemctl disable") {
		t.Fatalf("expected disable error, got %v", err)
	}
}

func TestOperations_FailWhenSystemdUnsupported(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		func(string) (string, error) { return "", exec.ErrNotFound },
		func(string, []byte, os.FileMode) error { return nil },
	)
	installer := NewUnitInstaller(&mockCommander{})

	ops := []struct {
		name string
		run  func(Installer) error
	}{
		{name: "remove", run: func(i Installer) error { return i.RemoveUnit() }},
		{name: "stop", run: func(i Installer) error { return i.StopUnit() }},
		{name: "start", run: func(i Installer) error { return i.StartUnit() }},
		{name: "enable", run: func(i Installer) error { return i.EnableUnit() }},
		{name: "disable", run: func(i Installer) error { return i.DisableUnit() }},
	}
	for _, tc := range ops {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run(installer)
			if err == nil || !strings.Contains(err.Error(), "systemd is not supported") {
				t.Fatalf("expected unsupported error, got %v", err)
			}
		})
	}
}

func TestRemoveUnit_StopsDisablesRemovesAndReloads(t *testing.T) {
	removedPath := ""
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	removePath = func(path string) error {
		removedPath = path
		return nil
	}
	cmd := &mockCommander{}
	installer := NewUnitInstaller(cmd)

	if err := installer.RemoveUnit(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removedPath != systemdUnitPath {
		t.Fatalf("remove path: got %q want %q", removedPath, systemdUnitPath)
	}
	if len(cmd.runCalls) != 3 {
		t.Fatalf("expected 3 systemctl calls, got %d", len(cmd.runCalls))
	}
	if cmd.runCalls[0] != [2]string{"systemctl", "stop"} {
		t.Fatalf("unexpected first command: %v", cmd.runCalls[0])
	}
	if cmd.runCalls[1] != [2]string{"systemctl", "disable"} {
		t.Fatalf("unexpected second command: %v", cmd.runCalls[1])
	}
	if cmd.runCalls[2] != [2]string{"systemctl", "daemon-reload"} {
		t.Fatalf("unexpected third command: %v", cmd.runCalls[2])
	}
}

func TestPrivilegedOperations_FailWithoutAdminRights(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	geteuid = func() int { return 1000 }

	operations := []struct {
		name string
		run  func(installer Installer) error
	}{
		{name: "start", run: func(installer Installer) error { return installer.StartUnit() }},
		{name: "stop", run: func(installer Installer) error { return installer.StopUnit() }},
		{name: "enable", run: func(installer Installer) error { return installer.EnableUnit() }},
		{name: "disable", run: func(installer Installer) error { return installer.DisableUnit() }},
		{name: "install-client", run: func(installer Installer) error {
			_, err := installer.InstallClientUnit()
			return err
		}},
		{name: "install-server", run: func(installer Installer) error {
			_, err := installer.InstallServerUnit()
			return err
		}},
		{name: "remove", run: func(installer Installer) error { return installer.RemoveUnit() }},
	}

	for _, tc := range operations {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &mockCommander{}
			installer := NewUnitInstaller(cmd)
			err := tc.run(installer)
			if err == nil {
				t.Fatalf("expected permission error")
			}
			if !strings.Contains(err.Error(), "admin privileges are required") {
				t.Fatalf("expected admin privileges error, got %v", err)
			}
			if len(cmd.runCalls) != 0 {
				t.Fatalf("expected no systemctl calls when not admin, got %v", cmd.runCalls)
			}
		})
	}
}

func TestStatus_NotInstalled_ReturnsDefaults(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{
		combinedOutputByArg: map[string]mockCombinedOutputResult{
			"is-enabled": {output: []byte("disabled\n"), err: commandExitError(t, 4)},
			"is-active":  {output: []byte("inactive\n"), err: commandExitError(t, 4)},
			"show": {
				output: []byte("LoadState=not-found\nActiveState=inactive\nSubState=dead\nResult=success\nExecMainStatus=0\nExecStart=\n"),
			},
		},
	}
	installer := NewUnitInstaller(cmd)

	status, err := installer.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Installed ||
		status.UnitFileState != UnitFileStateDisabled ||
		status.ActiveState != UnitActiveStateInactive ||
		status.LoadState != UnitLoadStateNotFound {
		t.Fatalf("expected empty status for missing unit, got %+v", status)
	}
	if status.Role != UnitRoleUnknown {
		t.Fatalf("expected unknown role, got %q", status.Role)
	}
	if len(cmd.combinedOutputCalls) == 0 || len(cmd.runCalls) != 0 {
		t.Fatalf("expected status probing systemctl calls for missing unit, got run=%v combined=%v", cmd.runCalls, cmd.combinedOutputCalls)
	}
}

func TestStatus_InstalledEnabledActiveClient(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
		func(string) ([]byte, error) { return []byte("ExecStart=tungo c\n"), nil },
	)
	cmd := &mockCommander{
		combinedOutputByArg: map[string]mockCombinedOutputResult{
			"is-enabled": {output: []byte("enabled\n")},
			"is-active":  {output: []byte("active\n")},
		},
	}
	installer := NewUnitInstaller(cmd)

	status, err := installer.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Installed ||
		status.UnitFileState != UnitFileStateEnabled ||
		status.ActiveState != UnitActiveStateActive {
		t.Fatalf("expected installed+enabled+active states, got %+v", status)
	}
	if status.Role != UnitRoleClient {
		t.Fatalf("expected client role, got %q", status.Role)
	}
}

func TestStatus_InstalledEnabledActivatingClient_IsNotActive(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
		func(string) ([]byte, error) { return []byte("ExecStart=tungo c\n"), nil },
	)
	cmd := &mockCommander{
		combinedOutputByArg: map[string]mockCombinedOutputResult{
			"is-enabled": {output: []byte("enabled\n")},
			"is-active":  {output: []byte("activating\n")},
		},
	}
	installer := NewUnitInstaller(cmd)

	status, err := installer.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.ActiveState != UnitActiveStateActivating {
		t.Fatalf("expected activating active-state, got %+v", status)
	}
}

func TestStatus_InstalledDisabledInactiveServer(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
		func(string) ([]byte, error) { return []byte("ExecStart=tungo s\n"), nil },
	)
	cmd := &mockCommander{
		combinedOutputByArg: map[string]mockCombinedOutputResult{
			"is-enabled": {output: []byte("disabled\n"), err: commandExitError(t, 1)},
			"is-active":  {output: []byte("inactive\n"), err: commandExitError(t, 3)},
		},
	}
	installer := NewUnitInstaller(cmd)

	status, err := installer.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Installed {
		t.Fatalf("expected installed status, got %+v", status)
	}
	if status.UnitFileState != UnitFileStateDisabled || status.ActiveState != UnitActiveStateInactive {
		t.Fatalf("expected disabled+inactive states, got %+v", status)
	}
	if status.Role != UnitRoleServer {
		t.Fatalf("expected server role, got %q", status.Role)
	}
}

func TestStatus_InstalledPreservesRawSystemdStates(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
		func(string) ([]byte, error) { return []byte("ExecStart=tungo c\n"), nil },
	)
	cmd := &mockCommander{
		combinedOutputByArg: map[string]mockCombinedOutputResult{
			"is-enabled": {output: []byte("static\n")},
			"is-active":  {output: []byte("deactivating\n")},
			"show": {
				output: []byte("LoadState=loaded\nSubState=stop-sigterm\nResult=exit-code\nExecMainStatus=203\nExecStart={ path=/usr/local/bin/tungo ; argv[]=/usr/local/bin/tungo s ; }\nFragmentPath=/usr/lib/systemd/system/tungo.service\n"),
			},
		},
	}
	installer := NewUnitInstaller(cmd)

	status, err := installer.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.UnitFileState != UnitFileState("static") {
		t.Fatalf("expected raw unit-file state static, got %q", status.UnitFileState)
	}
	if status.ActiveState != UnitActiveStateDeactivating {
		t.Fatalf("expected active-state deactivating, got %q", status.ActiveState)
	}
	if status.LoadState != UnitLoadStateLoaded ||
		status.SubState != "stop-sigterm" ||
		status.Result != "exit-code" ||
		status.ExecMainStatus != "203" ||
		status.ExecStart != "{ path=/usr/local/bin/tungo ; argv[]=/usr/local/bin/tungo s ; }" ||
		status.FragmentPath != "/usr/lib/systemd/system/tungo.service" {
		t.Fatalf("expected raw show properties, got %+v", status)
	}
	if status.Managed {
		t.Fatalf("expected unmanaged status for non-/etc unit fragment, got %+v", status)
	}
}

func TestStatus_ManagedTrueWhenFragmentPathInEtc(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
		func(string) ([]byte, error) { return []byte("ExecStart=tungo c\n"), nil },
	)
	cmd := &mockCommander{
		combinedOutputByArg: map[string]mockCombinedOutputResult{
			"is-enabled": {output: []byte("enabled\n")},
			"is-active":  {output: []byte("active\n")},
			"show": {
				output: []byte("LoadState=loaded\nActiveState=active\nSubState=running\nResult=success\nExecMainStatus=0\nExecStart=/usr/local/bin/tungo c\nFragmentPath=/etc/systemd/system/tungo.service\n"),
			},
		},
	}
	installer := NewUnitInstaller(cmd)

	status, err := installer.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Managed {
		t.Fatalf("expected managed status for /etc fragment path, got %+v", status)
	}
}

func TestStatus_FailsOnUnexpectedIsEnabledError(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			if name == "tungo" {
				return "/usr/local/bin/tungo", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
		func(string) ([]byte, error) { return []byte("ExecStart=tungo c\n"), nil },
	)
	cmd := &mockCommander{
		combinedOutputErr: errors.New("boom"),
	}
	installer := NewUnitInstaller(cmd)

	_, err := installer.Status()
	if err == nil || !strings.Contains(err.Error(), "is-enabled") {
		t.Fatalf("expected is-enabled error, got %v", err)
	}
}

func TestStatus_FailsWhenSystemdUnsupported(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		func(string) (string, error) { return "", exec.ErrNotFound },
		func(string, []byte, os.FileMode) error { return nil },
	)
	installer := NewUnitInstaller(&mockCommander{})
	_, err := installer.Status()
	if err == nil || !strings.Contains(err.Error(), "systemd is not supported") {
		t.Fatalf("expected unsupported error, got %v", err)
	}
}

func TestStatus_FailsWhenShowFails(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	installer := NewUnitInstaller(&mockCommander{
		combinedOutputByArg: map[string]mockCombinedOutputResult{
			"is-enabled": {output: []byte("enabled\n")},
			"is-active":  {output: []byte("active\n")},
			"show":       {err: errors.New("show failed")},
		},
	})
	_, err := installer.Status()
	if err == nil || !strings.Contains(err.Error(), "systemctl show") {
		t.Fatalf("expected show error, got %v", err)
	}
}

func TestStatus_FailsWhenIsActiveFails(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
		func(string) ([]byte, error) { return []byte("ExecStart=tungo c\n"), nil },
	)
	cmd := &mockCommander{
		combinedOutputByArg: map[string]mockCombinedOutputResult{
			"is-enabled": {output: []byte("enabled\n")},
			"is-active":  {err: errors.New("active failed")},
		},
	}
	installer := NewUnitInstaller(cmd)
	_, err := installer.Status()
	if err == nil || !strings.Contains(err.Error(), "is-active") {
		t.Fatalf("expected is-active error, got %v", err)
	}
}

func TestStatus_ReadUnitFailure_DoesNotFail(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
		func(string) ([]byte, error) { return nil, errors.New("read failed") },
	)
	cmd := &mockCommander{
		combinedOutputByArg: map[string]mockCombinedOutputResult{
			"is-enabled": {output: []byte("enabled\n")},
			"is-active":  {output: []byte("active\n")},
			"show": {
				output: []byte("LoadState=loaded\nActiveState=active\nSubState=running\nResult=success\nExecMainStatus=0\nExecStart=/usr/local/bin/tungo c\n"),
			},
		},
	}
	installer := NewUnitInstaller(cmd)
	status, err := installer.Status()
	if err != nil {
		t.Fatalf("expected status to succeed when unit file is unreadable, got %v", err)
	}
	if status.Role != UnitRoleClient {
		t.Fatalf("expected role from ExecStart fallback, got %q", status.Role)
	}
}

func TestRemoveUnit_FailsOnStopError(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{
		runErrByArg: map[string]error{
			"stop": errors.New("stop failed"),
		},
	}
	installer := NewUnitInstaller(cmd)
	err := installer.RemoveUnit()
	if err == nil || !strings.Contains(err.Error(), "systemctl stop") {
		t.Fatalf("expected stop error, got %v", err)
	}
}

func TestRemoveUnit_FailsOnDisableError(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{
		runErrByArg: map[string]error{
			"disable": errors.New("disable failed"),
		},
	}
	installer := NewUnitInstaller(cmd)
	err := installer.RemoveUnit()
	if err == nil || !strings.Contains(err.Error(), "systemctl disable") {
		t.Fatalf("expected disable error, got %v", err)
	}
}

func TestRemoveUnit_FailsOnRemoveError(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	removePath = func(string) error { return errors.New("remove failed") }
	installer := NewUnitInstaller(&mockCommander{})
	err := installer.RemoveUnit()
	if err == nil || !strings.Contains(err.Error(), "failed to remove") {
		t.Fatalf("expected remove error, got %v", err)
	}
}

func TestRemoveUnit_FailsOnDaemonReloadError(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{
		runErrByArg: map[string]error{
			"daemon-reload": errors.New("reload failed"),
		},
	}
	installer := NewUnitInstaller(cmd)
	err := installer.RemoveUnit()
	if err == nil || !strings.Contains(err.Error(), "daemon-reload") {
		t.Fatalf("expected daemon-reload error, got %v", err)
	}
}

func TestDetectUnitRole(t *testing.T) {
	if got := detectUnitRole("ExecStart=tungo c\n"); got != UnitRoleClient {
		t.Fatalf("expected client role, got %q", got)
	}
	if got := detectUnitRole("ExecStart=tungo s\n"); got != UnitRoleServer {
		t.Fatalf("expected server role, got %q", got)
	}
	if got := detectUnitRole("ExecStart=/usr/local/bin/tungo c\n"); got != UnitRoleClient {
		t.Fatalf("expected client role for absolute path, got %q", got)
	}
	if got := detectUnitRole("ExecStart=/usr/local/bin/tungo s\n"); got != UnitRoleServer {
		t.Fatalf("expected server role for absolute path, got %q", got)
	}
	if got := detectUnitRole("ExecStart=/usr/bin/env tungo s --foreground\n"); got != UnitRoleServer {
		t.Fatalf("expected server role for wrapped command, got %q", got)
	}
	if got := detectUnitRole("ExecStart=/usr/bin/env ABC=1 /usr/local/bin/tungo c --log-level debug\n"); got != UnitRoleClient {
		t.Fatalf("expected client role for command with extra args, got %q", got)
	}
	if got := detectUnitRole("ExecStart=/usr/bin/other\n"); got != UnitRoleUnknown {
		t.Fatalf("expected unknown role, got %q", got)
	}
	if got := detectUnitRole("ExecStart=\n"); got != UnitRoleUnknown {
		t.Fatalf("expected unknown role for empty exec start, got %q", got)
	}
	if got := detectUnitRoleFromExecStart("{ path=/usr/local/bin/tungo ; argv[]=/usr/local/bin/tungo ; argv[]=s ; }"); got != UnitRoleServer {
		t.Fatalf("expected server role for systemctl show ExecStart, got %q", got)
	}
	if got := detectUnitRoleFromExecStart("{ path=/usr/local/bin/tungo ; argv[]=/usr/local/bin/tungo ; argv[]=c ; }"); got != UnitRoleClient {
		t.Fatalf("expected client role for systemctl show ExecStart, got %q", got)
	}
}

func TestInstallUnit_FailsWhenLstatReturnsUnexpectedError(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	lstatPath = func(path string) (os.FileInfo, error) {
		if path == tungoBinaryPath {
			return nil, errors.New("lstat failed")
		}
		return nil, nil
	}
	installer := NewUnitInstaller(&mockCommander{})
	_, err := installer.InstallClientUnit()
	if err == nil || !strings.Contains(err.Error(), "failed to lstat") {
		t.Fatalf("expected lstat error, got %v", err)
	}
}

func TestInstallUnit_FailsWhenLstatReturnsNilInfo(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	lstatPath = func(path string) (os.FileInfo, error) {
		if path == tungoBinaryPath {
			return nil, nil
		}
		return nil, nil
	}
	installer := NewUnitInstaller(&mockCommander{})
	_, err := installer.InstallClientUnit()
	if err == nil || !strings.Contains(err.Error(), "empty file info") {
		t.Fatalf("expected empty file info error, got %v", err)
	}
}

func TestInstallUnit_FailsWhenTungoBinaryIsNotRegular(t *testing.T) {
	withSystemdHooks(
		t,
		func(path string) (os.FileInfo, error) {
			if path == tungoBinaryPath {
				return mockFileInfo{name: "tungo", mode: os.ModeDir | 0o755, sys: statWithUID(0)}, nil
			}
			return nil, nil
		},
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	installer := NewUnitInstaller(&mockCommander{})
	_, err := installer.InstallClientUnit()
	if err == nil || !strings.Contains(err.Error(), "is not a regular file") {
		t.Fatalf("expected non-regular file error, got %v", err)
	}
}

func TestInstallUnit_FailsWhenOwnerCannotBeVerified(t *testing.T) {
	withSystemdHooks(
		t,
		func(path string) (os.FileInfo, error) {
			if path == tungoBinaryPath {
				return mockFileInfo{name: "tungo", mode: 0o755, sys: nil}, nil
			}
			return nil, nil
		},
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/bin/systemctl", nil
			}
			return "", exec.ErrNotFound
		},
		func(string, []byte, os.FileMode) error { return nil },
	)
	installer := NewUnitInstaller(&mockCommander{})
	_, err := installer.InstallClientUnit()
	if err == nil || !strings.Contains(err.Error(), "failed to verify owner") {
		t.Fatalf("expected owner verification error, got %v", err)
	}
}

func commandExitError(t *testing.T, code int) error {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestCommandExitHelperProcess")
	cmd.Env = append(
		os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		fmt.Sprintf("GO_HELPER_EXIT_CODE=%d", code),
	)
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit error for code %d", code)
	}
	return err
}

func TestCommandExitHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	code, err := strconv.Atoi(os.Getenv("GO_HELPER_EXIT_CODE"))
	if err != nil {
		os.Exit(2)
	}
	os.Exit(code)
}
