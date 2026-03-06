package systemd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type mockCommander struct {
	runCalls            [][2]string
	runErr              error
	combinedOutputCalls [][2]string
	combinedOutput      []byte
	combinedOutputErr   error
}

func (m *mockCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	arg0 := ""
	if len(args) > 0 {
		arg0 = args[0]
	}
	m.combinedOutputCalls = append(m.combinedOutputCalls, [2]string{name, arg0})
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
	prevLook := lookPath
	prevWrite := writeFilePath
	prevRead := readFilePath
	prevGeteuid := geteuid
	readHook := func(string) ([]byte, error) { return []byte(""), nil }
	if len(read) > 0 && read[0] != nil {
		readHook = read[0]
	}
	statPath = stat
	lookPath = look
	writeFilePath = write
	readFilePath = readHook
	geteuid = func() int { return 0 }
	t.Cleanup(func() {
		statPath = prevStat
		lookPath = prevLook
		writeFilePath = prevWrite
		readFilePath = prevRead
		geteuid = prevGeteuid
	})
}

func TestSupported_TrueWhenRuntimeDirAndSystemctlExist(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(string) (string, error) { return "/bin/systemctl", nil },
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
		func(string) (string, error) { return "/bin/systemctl", nil },
		func(string, []byte, os.FileMode) error { return nil },
	)
	installer := NewUnitInstaller(&mockCommander{})
	if installer.Supported() {
		t.Fatal("expected Supported()=false when /run/systemd/system is missing")
	}
}

func TestInstallServerUnit_WritesServerModeAndEnablesService(t *testing.T) {
	var gotPath string
	var gotContent string
	var gotPerm os.FileMode
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(string) (string, error) { return "/bin/systemctl", nil },
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
	if !strings.Contains(gotContent, "ExecStart=tungo s") {
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
		func(string) (string, error) { return "/bin/systemctl", nil },
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
	if !strings.Contains(gotContent, "ExecStart=tungo c") {
		t.Fatalf("expected client unit content, got %q", gotContent)
	}
}

func TestInstallUnit_FailsWhenSystemctlCommandFails(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(string) (string, error) { return "/bin/systemctl", nil },
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{runErr: errors.New("boom")}
	installer := NewUnitInstaller(cmd)

	_, err := installer.InstallServerUnit()
	if err == nil || !strings.Contains(err.Error(), "daemon-reload") {
		t.Fatalf("expected daemon-reload error, got %v", err)
	}
}

func TestIsUnitActive_ReturnsTrueWhenServiceIsActive(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(string) (string, error) { return "/bin/systemctl", nil },
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{}
	installer := NewUnitInstaller(cmd)

	active, err := installer.IsUnitActive()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Fatal("expected active=true")
	}
	if len(cmd.runCalls) != 1 || cmd.runCalls[0] != [2]string{"systemctl", "is-active"} {
		t.Fatalf("unexpected run calls: %v", cmd.runCalls)
	}
}

func TestIsUnitActive_ReturnsFalseWhenServiceIsInactive(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(string) (string, error) { return "/bin/systemctl", nil },
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{runErr: commandExitError(t, 3)}
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
		func(string) (string, error) { return "/bin/systemctl", nil },
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{runErr: commandExitError(t, 4)}
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
		func(string) (string, error) { return "/bin/systemctl", nil },
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{runErr: errors.New("boom")}
	installer := NewUnitInstaller(cmd)

	_, err := installer.IsUnitActive()
	if err == nil || !strings.Contains(err.Error(), "is-active") {
		t.Fatalf("expected is-active error, got %v", err)
	}
}

func TestStopUnit_RunsSystemctlStop(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(string) (string, error) { return "/bin/systemctl", nil },
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
		func(string) (string, error) { return "/bin/systemctl", nil },
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
		func(string) (string, error) { return "/bin/systemctl", nil },
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

func TestEnableUnit_RunsSystemctlEnable(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(string) (string, error) { return "/bin/systemctl", nil },
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

func TestDisableUnit_RunsSystemctlDisable(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(string) (string, error) { return "/bin/systemctl", nil },
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

func TestPrivilegedOperations_FailWithoutAdminRights(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(string) (string, error) { return "/bin/systemctl", nil },
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
		func(path string) (os.FileInfo, error) {
			if path == systemdUnitPath {
				return nil, os.ErrNotExist
			}
			return nil, nil
		},
		func(string) (string, error) { return "/bin/systemctl", nil },
		func(string, []byte, os.FileMode) error { return nil },
	)
	cmd := &mockCommander{}
	installer := NewUnitInstaller(cmd)

	status, err := installer.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Installed || status.Enabled || status.Active {
		t.Fatalf("expected empty status for missing unit, got %+v", status)
	}
	if status.Role != UnitRoleUnknown {
		t.Fatalf("expected unknown role, got %q", status.Role)
	}
	if len(cmd.combinedOutputCalls) != 0 || len(cmd.runCalls) != 0 {
		t.Fatalf("expected no systemctl calls for missing unit, got run=%v combined=%v", cmd.runCalls, cmd.combinedOutputCalls)
	}
}

func TestStatus_InstalledEnabledActiveClient(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(string) (string, error) { return "/bin/systemctl", nil },
		func(string, []byte, os.FileMode) error { return nil },
		func(string) ([]byte, error) { return []byte("ExecStart=tungo c\n"), nil },
	)
	cmd := &mockCommander{
		combinedOutput: []byte("enabled\n"),
	}
	installer := NewUnitInstaller(cmd)

	status, err := installer.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Installed || !status.Enabled || !status.Active {
		t.Fatalf("expected installed+enabled+active, got %+v", status)
	}
	if status.Role != UnitRoleClient {
		t.Fatalf("expected client role, got %q", status.Role)
	}
}

func TestStatus_InstalledDisabledInactiveServer(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(string) (string, error) { return "/bin/systemctl", nil },
		func(string, []byte, os.FileMode) error { return nil },
		func(string) ([]byte, error) { return []byte("ExecStart=tungo s\n"), nil },
	)
	cmd := &mockCommander{
		runErr:            commandExitError(t, 3),
		combinedOutput:    []byte("disabled\n"),
		combinedOutputErr: commandExitError(t, 1),
	}
	installer := NewUnitInstaller(cmd)

	status, err := installer.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Installed {
		t.Fatalf("expected installed status, got %+v", status)
	}
	if status.Enabled || status.Active {
		t.Fatalf("expected disabled+inactive, got %+v", status)
	}
	if status.Role != UnitRoleServer {
		t.Fatalf("expected server role, got %q", status.Role)
	}
}

func TestStatus_FailsOnUnexpectedIsEnabledError(t *testing.T) {
	withSystemdHooks(
		t,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(string) (string, error) { return "/bin/systemctl", nil },
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

func TestDetectUnitRole(t *testing.T) {
	if got := detectUnitRole("ExecStart=tungo c\n"); got != UnitRoleClient {
		t.Fatalf("expected client role, got %q", got)
	}
	if got := detectUnitRole("ExecStart=tungo s\n"); got != UnitRoleServer {
		t.Fatalf("expected server role, got %q", got)
	}
	if got := detectUnitRole("ExecStart=/usr/bin/other\n"); got != UnitRoleUnknown {
		t.Fatalf("expected unknown role, got %q", got)
	}
}

func commandExitError(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	if err == nil {
		t.Fatalf("expected non-zero exit error for code %d", code)
	}
	return err
}
