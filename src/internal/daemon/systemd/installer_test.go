package systemd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"tungo/internal/mode"
	"tungo/internal/platform/command"
)

type systemdCommandResult struct {
	output []byte
	err    error
}

type systemdRunnerFunc func(context.Context, string, ...string) ([]byte, error)

func (f systemdRunnerFunc) CombinedOutputContext(ctx context.Context, name string, args ...string) ([]byte, error) {
	return f(ctx, name, args...)
}

func TestSystemctl_DefaultDeadlineAndNoninteractiveExecution(t *testing.T) {
	installer := newSystemdTestInstaller(t, &systemdTestRunner{})
	installer.runner = systemdRunnerFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "systemctl" || !strings.Contains(strings.Join(args, " "), "--no-ask-password --no-pager") {
			t.Fatalf("command can interact with the TUI terminal: %s %v", name, args)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	})
	synctest.Test(t, func(t *testing.T) {
		if err := installer.StartUnit(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("blocked systemctl should time out, got %v", err)
		}
	})
}

func TestSystemctl_CancelsRunningProcess(t *testing.T) {
	for _, operation := range []string{"start", "status"} {
		t.Run(operation, func(t *testing.T) {
			installer := newSystemdTestInstaller(t, &systemdTestRunner{})
			installer.runner = command.New()
			// This substitutes only the executable; the production runner starts,
			// cancels and reaps a real subprocess.
			path := filepath.Join(os.Getenv("PATH"), "systemctl")
			marker := filepath.Join(t.TempDir(), "started")
			t.Setenv("TUNGO_SYSTEMCTL_STARTED", marker)
			if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf started > \"$TUNGO_SYSTEMCTL_STARTED\"\nexec /bin/sleep 30\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			started := time.Now()
			var err error
			if operation == "start" {
				err = installer.StartUnit(ctx)
			} else {
				_, err = installer.Status(ctx)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected deadline error, got %v", err)
			}
			if _, err := os.Stat(marker); err != nil {
				t.Fatalf("subprocess did not start before cancellation: %v", err)
			}
			if time.Since(started) > 2*time.Second {
				t.Fatal("command did not stop promptly after cancellation")
			}
		})
	}
}

type systemdTestRunner struct {
	runs        []string
	queries     []string
	runErrors   map[string]error
	queryResult map[string]systemdCommandResult
}

func (c *systemdTestRunner) CombinedOutputContext(_ context.Context, _ string, args ...string) ([]byte, error) {
	operation := firstSystemdArg(args)
	switch operation {
	case "is-active", "is-enabled", "show":
		c.queries = append(c.queries, operation)
		result := c.queryResult[operation]
		return result.output, result.err
	default:
		c.runs = append(c.runs, operation)
		return nil, c.runErrors[operation]
	}
}

func firstSystemdArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func newSystemdTestInstaller(t *testing.T, runner *systemdTestRunner) *UnitInstaller {
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
	installer := NewUnitInstaller(runner)
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
	if installer.runner == nil {
		t.Fatal("NewUnitInstaller(nil) returned a nil runner")
	}
	if installer.config != DefaultConfig() {
		t.Fatalf("config = %+v, want %+v", installer.config, DefaultConfig())
	}
}

func TestSetup_InactiveUnit(t *testing.T) {
	runner := &systemdTestRunner{queryResult: map[string]systemdCommandResult{
		"is-active": {output: []byte("inactive\n")},
	}}
	installer := newSystemdTestInstaller(t, runner)

	path, err := installer.Setup(context.Background(), mode.Server)
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
	if want := []string{"daemon-reload", "enable"}; !reflect.DeepEqual(runner.runs, want) {
		t.Fatalf("systemctl calls = %v, want %v", runner.runs, want)
	}
}

func TestSetup_ActiveUnit(t *testing.T) {
	runner := &systemdTestRunner{queryResult: map[string]systemdCommandResult{
		"is-active": {output: []byte("active\n")},
	}}
	installer := newSystemdTestInstaller(t, runner)

	if _, err := installer.Setup(context.Background(), mode.Client); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	want := []string{"stop", "daemon-reload", "enable", "start"}
	if !reflect.DeepEqual(runner.runs, want) {
		t.Fatalf("systemctl calls = %v, want %v", runner.runs, want)
	}
}

func TestSetup_RejectsInvalidModeBeforeIO(t *testing.T) {
	runner := &systemdTestRunner{}
	installer := NewUnitInstaller(runner)

	if _, err := installer.Setup(context.Background(), 0); err == nil || !strings.Contains(err.Error(), "invalid daemon mode") {
		t.Fatalf("Setup() error = %v", err)
	}
	if len(runner.runs) != 0 || len(runner.queries) != 0 {
		t.Fatalf("unexpected systemctl calls: runs=%v queries=%v", runner.runs, runner.queries)
	}
}

func TestSetup_RestartsActiveUnitAfterWriteError(t *testing.T) {
	runner := &systemdTestRunner{queryResult: map[string]systemdCommandResult{
		"is-active": {output: []byte("active\n")},
	}}
	installer := newSystemdTestInstaller(t, runner)
	installer.config.UnitPath = filepath.Join(t.TempDir(), "missing", "tungo.service")

	if _, err := installer.Setup(context.Background(), mode.Server); err == nil {
		t.Fatal("Setup() error = nil")
	}
	want := []string{"stop", "start"}
	if !reflect.DeepEqual(runner.runs, want) {
		t.Fatalf("systemctl calls = %v, want %v", runner.runs, want)
	}
}

func TestInstallUnit_EnableFailureRollsBackFile(t *testing.T) {
	enableErr := errors.New("enable failed")
	runner := &systemdTestRunner{runErrors: map[string]error{"enable": enableErr}}
	installer := newSystemdTestInstaller(t, runner)

	_, err := installer.installUnit(context.Background(), []string{"c"})
	if !errors.Is(err, enableErr) {
		t.Fatalf("installUnit() error = %v", err)
	}
	if _, statErr := os.Stat(installer.config.UnitPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rolled-back unit stat error = %v", statErr)
	}
	want := []string{"daemon-reload", "enable", "daemon-reload"}
	if !reflect.DeepEqual(runner.runs, want) {
		t.Fatalf("systemctl calls = %v, want %v", runner.runs, want)
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
			runner := &systemdTestRunner{queryResult: map[string]systemdCommandResult{
				"is-active": {output: []byte(tt.state)},
			}}
			installer := newSystemdTestInstaller(t, runner)
			got, err := installer.IsUnitActive(context.Background())
			if err != nil || got != tt.want {
				t.Fatalf("IsUnitActive() = (%v, %v), want (%v, nil)", got, err, tt.want)
			}
		})
	}
}

func TestUnitOperations(t *testing.T) {
	tests := []struct {
		name string
		call func(*UnitInstaller, context.Context) error
		want string
	}{
		{name: "start", call: (*UnitInstaller).StartUnit, want: "start"},
		{name: "stop", call: (*UnitInstaller).StopUnit, want: "stop"},
		{name: "enable", call: (*UnitInstaller).EnableUnit, want: "enable"},
		{name: "disable", call: (*UnitInstaller).DisableUnit, want: "disable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &systemdTestRunner{}
			installer := newSystemdTestInstaller(t, runner)
			if err := tt.call(installer, context.Background()); err != nil {
				t.Fatalf("%s error = %v", tt.name, err)
			}
			if want := []string{tt.want}; !reflect.DeepEqual(runner.runs, want) {
				t.Fatalf("systemctl calls = %v, want %v", runner.runs, want)
			}
		})
	}
}

func TestRemoveUnit(t *testing.T) {
	runner := &systemdTestRunner{}
	installer := newSystemdTestInstaller(t, runner)
	if err := os.WriteFile(installer.config.UnitPath, []byte("unit"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installer.RemoveUnit(context.Background()); err != nil {
		t.Fatalf("RemoveUnit() error = %v", err)
	}
	if _, err := os.Stat(installer.config.UnitPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unit stat error = %v", err)
	}
	want := []string{"stop", "disable", "daemon-reload"}
	if !reflect.DeepEqual(runner.runs, want) {
		t.Fatalf("systemctl calls = %v, want %v", runner.runs, want)
	}
}

func TestStatus(t *testing.T) {
	runner := &systemdTestRunner{queryResult: map[string]systemdCommandResult{
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
	installer := newSystemdTestInstaller(t, runner)
	installer.config.UnitPath = "/etc/systemd/system/tungo.service"

	status, err := installer.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Installed || !status.Managed || status.Role != UnitRoleClient {
		t.Fatalf("Status() = %+v", status)
	}
}

func TestStatus_DetectsRoleFromManagedUnitFile(t *testing.T) {
	runner := &systemdTestRunner{queryResult: map[string]systemdCommandResult{
		"is-enabled": {output: []byte("enabled\n")},
		"is-active":  {output: []byte("inactive\n")},
	}}
	installer := newSystemdTestInstaller(t, runner)
	runner.queryResult["show"] = systemdCommandResult{output: []byte(
		"LoadState=loaded\nActiveState=inactive\nFragmentPath=" + installer.config.UnitPath + "\n",
	)}
	if err := os.WriteFile(
		installer.config.UnitPath,
		[]byte(UnitFileContent("/usr/local/bin/tungo", []string{"s"})),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	status, err := installer.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Role != UnitRoleServer {
		t.Fatalf("Status().Role = %q, want %q", status.Role, UnitRoleServer)
	}
}
