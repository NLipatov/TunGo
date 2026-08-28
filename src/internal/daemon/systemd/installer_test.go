package systemd

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"tungo/internal/mode"
)

type systemdCommandResult struct {
	output []byte
	err    error
}

type systemdTestCommander struct {
	runs        []string
	queries     []string
	runErrors   map[string]error
	queryResult map[string]systemdCommandResult
}

func (c *systemdTestCommander) Run(_ string, args ...string) error {
	operation := firstSystemdArg(args)
	c.runs = append(c.runs, operation)
	return c.runErrors[operation]
}

func (c *systemdTestCommander) Output(_ string, args ...string) ([]byte, error) {
	return c.CombinedOutput("systemctl", args...)
}

func (c *systemdTestCommander) CombinedOutput(_ string, args ...string) ([]byte, error) {
	operation := firstSystemdArg(args)
	c.queries = append(c.queries, operation)
	result := c.queryResult[operation]
	return result.output, result.err
}

func firstSystemdArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func newSystemdTestInstaller(t *testing.T, commander *systemdTestCommander) *UnitInstaller {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("systemd executable ownership is Unix-specific")
	}

	binDir := t.TempDir()
	systemctlPath := filepath.Join(binDir, "systemctl")
	if err := os.WriteFile(systemctlPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	binaryPath := rootOwnedExecutable(t)
	runtimeDir := t.TempDir()
	installer := NewUnitInstaller(commander)
	installer.config = Config{
		RuntimeDir: runtimeDir,
		UnitPath:   filepath.Join(runtimeDir, "tungo.service"),
		UnitName:   "tungo.service",
		BinaryPath: binaryPath,
	}
	return installer
}

func rootOwnedExecutable(t *testing.T) string {
	t.Helper()
	for _, path := range []string{"/bin/sh", "/usr/bin/true", "/bin/true"} {
		if validateTungoBinaryForSystemd(path) == nil {
			return path
		}
	}
	t.Skip("no root-owned executable available")
	return ""
}

func TestNewUnitInstaller_Defaults(t *testing.T) {
	installer := NewUnitInstaller(nil)
	if installer.commander == nil {
		t.Fatal("NewUnitInstaller(nil) returned a nil commander")
	}
	if installer.config != DefaultConfig() {
		t.Fatalf("config = %+v, want %+v", installer.config, DefaultConfig())
	}
}

func TestSetup_InactiveUnit(t *testing.T) {
	commander := &systemdTestCommander{queryResult: map[string]systemdCommandResult{
		"is-active": {output: []byte("inactive\n")},
	}}
	installer := newSystemdTestInstaller(t, commander)

	path, err := installer.Setup(mode.Server)
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if path != installer.config.UnitPath {
		t.Fatalf("Setup() path = %q, want %q", path, installer.config.UnitPath)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), " s") {
		t.Fatalf("server unit body = %q", body)
	}
	if want := []string{"daemon-reload", "enable"}; !reflect.DeepEqual(commander.runs, want) {
		t.Fatalf("systemctl calls = %v, want %v", commander.runs, want)
	}
}

func TestSetup_ActiveUnit(t *testing.T) {
	commander := &systemdTestCommander{queryResult: map[string]systemdCommandResult{
		"is-active": {output: []byte("active\n")},
	}}
	installer := newSystemdTestInstaller(t, commander)

	if _, err := installer.Setup(mode.Client); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	want := []string{"stop", "daemon-reload", "enable", "start"}
	if !reflect.DeepEqual(commander.runs, want) {
		t.Fatalf("systemctl calls = %v, want %v", commander.runs, want)
	}
}

func TestSetup_RejectsInvalidModeBeforeIO(t *testing.T) {
	commander := &systemdTestCommander{}
	installer := NewUnitInstaller(commander)

	if _, err := installer.Setup(0); err == nil || !strings.Contains(err.Error(), "invalid daemon mode") {
		t.Fatalf("Setup() error = %v", err)
	}
	if len(commander.runs) != 0 || len(commander.queries) != 0 {
		t.Fatalf("unexpected systemctl calls: runs=%v queries=%v", commander.runs, commander.queries)
	}
}

func TestSetup_RestartsActiveUnitAfterWriteError(t *testing.T) {
	commander := &systemdTestCommander{queryResult: map[string]systemdCommandResult{
		"is-active": {output: []byte("active\n")},
	}}
	installer := newSystemdTestInstaller(t, commander)
	installer.config.UnitPath = filepath.Join(t.TempDir(), "missing", "tungo.service")

	if _, err := installer.Setup(mode.Server); err == nil {
		t.Fatal("Setup() error = nil")
	}
	want := []string{"stop", "start"}
	if !reflect.DeepEqual(commander.runs, want) {
		t.Fatalf("systemctl calls = %v, want %v", commander.runs, want)
	}
}

func TestInstallUnit_EnableFailureRollsBackFile(t *testing.T) {
	enableErr := errors.New("enable failed")
	commander := &systemdTestCommander{runErrors: map[string]error{"enable": enableErr}}
	installer := newSystemdTestInstaller(t, commander)

	_, err := installer.installUnit([]string{"c"})
	if !errors.Is(err, enableErr) {
		t.Fatalf("installUnit() error = %v", err)
	}
	if _, statErr := os.Stat(installer.config.UnitPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rolled-back unit stat error = %v", statErr)
	}
	want := []string{"daemon-reload", "enable", "daemon-reload"}
	if !reflect.DeepEqual(commander.runs, want) {
		t.Fatalf("systemctl calls = %v, want %v", commander.runs, want)
	}
}

func TestIsUnitActive(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{state: "active\n", want: true},
		{state: "activating\n", want: true},
		{state: "deactivating\n", want: true},
		{state: "inactive\n", want: false},
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.state), func(t *testing.T) {
			commander := &systemdTestCommander{queryResult: map[string]systemdCommandResult{
				"is-active": {output: []byte(tt.state)},
			}}
			installer := newSystemdTestInstaller(t, commander)
			got, err := installer.IsUnitActive()
			if err != nil || got != tt.want {
				t.Fatalf("IsUnitActive() = (%v, %v), want (%v, nil)", got, err, tt.want)
			}
		})
	}
}

func TestUnitOperations(t *testing.T) {
	tests := []struct {
		name string
		call func(*UnitInstaller) error
		want string
	}{
		{name: "start", call: (*UnitInstaller).StartUnit, want: "start"},
		{name: "stop", call: (*UnitInstaller).StopUnit, want: "stop"},
		{name: "enable", call: (*UnitInstaller).EnableUnit, want: "enable"},
		{name: "disable", call: (*UnitInstaller).DisableUnit, want: "disable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commander := &systemdTestCommander{}
			installer := newSystemdTestInstaller(t, commander)
			if err := tt.call(installer); err != nil {
				t.Fatalf("%s error = %v", tt.name, err)
			}
			if want := []string{tt.want}; !reflect.DeepEqual(commander.runs, want) {
				t.Fatalf("systemctl calls = %v, want %v", commander.runs, want)
			}
		})
	}
}

func TestRemoveUnit(t *testing.T) {
	commander := &systemdTestCommander{}
	installer := newSystemdTestInstaller(t, commander)
	if err := os.WriteFile(installer.config.UnitPath, []byte("unit"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installer.RemoveUnit(); err != nil {
		t.Fatalf("RemoveUnit() error = %v", err)
	}
	if _, err := os.Stat(installer.config.UnitPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unit stat error = %v", err)
	}
	want := []string{"stop", "disable", "daemon-reload"}
	if !reflect.DeepEqual(commander.runs, want) {
		t.Fatalf("systemctl calls = %v, want %v", commander.runs, want)
	}
}

func TestStatus(t *testing.T) {
	commander := &systemdTestCommander{queryResult: map[string]systemdCommandResult{
		"is-enabled": {output: []byte("enabled\n")},
		"is-active":  {output: []byte("active\n")},
		"show": {output: []byte(strings.Join([]string{
			"LoadState=loaded",
			"ActiveState=active",
			"SubState=running",
			"Result=success",
			"ExecMainStatus=0",
			"ExecStart={ path=/usr/local/bin/tungo ; argv[]=/usr/local/bin/tungo c ; }",
			"FragmentPath=/etc/systemd/system/tungo.service",
		}, "\n"))},
	}}
	installer := newSystemdTestInstaller(t, commander)
	installer.config.UnitPath = "/etc/systemd/system/tungo.service"

	status, err := installer.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Installed || !status.Managed || status.Role != UnitRoleClient {
		t.Fatalf("Status() = %+v", status)
	}
}

func TestStatus_DetectsRoleFromManagedUnitFile(t *testing.T) {
	commander := &systemdTestCommander{queryResult: map[string]systemdCommandResult{
		"is-enabled": {output: []byte("enabled\n")},
		"is-active":  {output: []byte("inactive\n")},
	}}
	installer := newSystemdTestInstaller(t, commander)
	commander.queryResult["show"] = systemdCommandResult{output: []byte(
		"LoadState=loaded\nActiveState=inactive\nFragmentPath=" + installer.config.UnitPath + "\n",
	)}
	if err := os.WriteFile(
		installer.config.UnitPath,
		[]byte(UnitFileContent("/usr/local/bin/tungo", []string{"s"})),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	status, err := installer.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Role != UnitRoleServer {
		t.Fatalf("Status().Role = %q, want %q", status.Role, UnitRoleServer)
	}
}
