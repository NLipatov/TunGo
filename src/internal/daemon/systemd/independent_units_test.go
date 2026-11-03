package systemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"tungo/internal/mode"
)

func TestSetup_ClientAndServerHaveIndependentUnits(t *testing.T) {
	installer := newSystemdTestInstaller(t, &systemdTestRunner{})
	clientPath, err := installer.Setup(context.Background(), mode.Client)
	if err != nil {
		t.Fatal(err)
	}
	clientBody, err := os.ReadFile(clientPath)
	if err != nil {
		t.Fatal(err)
	}
	serverPath, err := installer.Setup(context.Background(), mode.Server)
	if err != nil {
		t.Fatal(err)
	}
	if clientPath == serverPath {
		t.Fatal("server setup replaced the client unit")
	}
	if _, err := installer.Setup(context.Background(), mode.Client); err == nil {
		t.Fatal("setup must reject an existing unit")
	}
	after, err := os.ReadFile(clientPath)
	if err != nil || string(after) != string(clientBody) {
		t.Fatalf("client unit changed: %q, %v", after, err)
	}
	serverBody, err := os.ReadFile(serverPath)
	if err != nil || !strings.Contains(string(serverBody), "\" s\n") {
		t.Fatalf("server unit: %q, %v", serverBody, err)
	}
}

func installedTestUnit(t *testing.T, installer *UnitInstaller, runner *systemdTestRunner, name, arg string) []byte {
	t.Helper()
	body := []byte(UnitFileContent("/usr/local/bin/tungo", []string{arg}))
	if err := os.WriteFile(installer.unitPath(name), body, 0600); err != nil {
		t.Fatal(err)
	}
	if runner.queryResult == nil {
		runner.queryResult = make(map[string]systemdCommandResult)
	}
	runner.queryResult["show "+name] = systemdCommandResult{output: []byte(fmt.Sprintf("LoadState=loaded\nActiveState=active\nExecStart=%s %s\nFragmentPath=%s\n", "/usr/local/bin/tungo", arg, installer.unitPath(name)))}
	runner.queryResult["is-enabled "+name] = systemdCommandResult{output: []byte("enabled\n")}
	return body
}

func TestLegacyUnit_RemainsManageableAlongsideOtherRole(t *testing.T) {
	for _, tc := range []struct {
		role, other    mode.Mode
		arg, otherName string
	}{
		{mode.Client, mode.Server, "c", "tungo-server.service"},
		{mode.Server, mode.Client, "s", "tungo-client.service"},
	} {
		t.Run(tc.arg, func(t *testing.T) {
			runner := &systemdTestRunner{}
			installer := newSystemdTestInstaller(t, runner)
			original := installedTestUnit(t, installer, runner, "tungo.service", tc.arg)
			ctx := context.Background()
			status, err := installer.Status(ctx, tc.role)
			if err != nil || status.Name != "tungo.service" || !status.Managed {
				t.Fatalf("legacy status: %+v, %v", status, err)
			}
			if active, err := installer.IsUnitActive(ctx, tc.other); err != nil || active {
				t.Fatalf("other role blocked by legacy unit: %v, %v", active, err)
			}
			if _, err := installer.Setup(ctx, tc.role); err == nil {
				t.Fatal("setup accepted an already installed role")
			}
			path, err := installer.Setup(ctx, tc.other)
			if err != nil || path != installer.unitPath(tc.otherName) {
				t.Fatalf("other setup: %q, %v", path, err)
			}
			otherBody, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(installer.unitPath("tungo.service"))
			if err != nil || string(after) != string(original) {
				t.Fatalf("legacy changed: %q, %v", after, err)
			}
			runner.calls = nil
			for _, action := range []func(context.Context, mode.Mode) error{installer.StartUnit, installer.StopUnit, installer.EnableUnit, installer.DisableUnit, installer.RemoveUnit} {
				if err := action(ctx, tc.role); err != nil {
					t.Fatal(err)
				}
			}
			for _, call := range runner.calls {
				switch call[0] {
				case "start", "stop", "enable", "disable":
					if call[1] != "tungo.service" {
						t.Fatalf("legacy operation affected %v", call)
					}
				}
			}
			if _, err := os.Stat(installer.unitPath("tungo.service")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("legacy file remains: %v", err)
			}
			after, err = os.ReadFile(path)
			if err != nil || string(after) != string(otherBody) {
				t.Fatalf("other role changed: %q, %v", after, err)
			}
			delete(runner.queryResult, "show tungo.service")
			delete(runner.queryResult, "is-enabled tungo.service")
			newPath, err := installer.Setup(ctx, tc.role)
			if err != nil || newPath == installer.unitPath("tungo.service") {
				t.Fatalf("reinstall did not use new name: %s, %v", newPath, err)
			}
		})
	}
}

func TestSetupAndStart_RejectAmbiguousLegacyUnit(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprint(unknown), func(t *testing.T) {
			runner := &systemdTestRunner{}
			installer := newSystemdTestInstaller(t, runner)
			arg := "c"
			if unknown {
				arg = "unrecognized"
			}
			installedTestUnit(t, installer, runner, "tungo.service", arg)
			if !unknown {
				installedTestUnit(t, installer, runner, "tungo-client.service", "c")
			}
			if _, err := installer.Setup(context.Background(), mode.Client); err == nil {
				t.Fatal("ambiguous setup accepted")
			}
			if err := installer.StartUnit(context.Background(), mode.Client); err == nil {
				t.Fatal("ambiguous start accepted")
			}
			if len(runner.runs) != 0 {
				t.Fatalf("ambiguous units were mutated: %v", runner.runs)
			}
		})
	}
}

func TestSetup_CancellationCleansOnlyNewUnit(t *testing.T) {
	for _, phase := range []string{"daemon-reload", "enable"} {
		t.Run(phase, func(t *testing.T) {
			runner := &systemdTestRunner{}
			installer := newSystemdTestInstaller(t, runner)
			serverBody := installedTestUnit(t, installer, runner, "tungo-server.service", "s")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cancelled := false
			installer.runner = systemdRunnerFunc(func(callCtx context.Context, name string, args ...string) ([]byte, error) {
				if err := callCtx.Err(); err != nil {
					return nil, err
				}
				if cancelled {
					if _, ok := callCtx.Deadline(); !ok {
						t.Fatal("cleanup has no deadline")
					}
				}
				output, err := runner.CombinedOutputContext(callCtx, name, args...)
				if args[0] == phase && !cancelled {
					cancelled = true
					cancel()
					return output, ctx.Err()
				}
				return output, err
			})
			if _, err := installer.Setup(ctx, mode.Client); !errors.Is(err, context.Canceled) {
				t.Fatalf("expected cancellation: %v", err)
			}
			if _, err := os.Stat(installer.unitPath("tungo-client.service")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("partial installation remains: %v", err)
			}
			after, err := os.ReadFile(installer.unitPath("tungo-server.service"))
			if err != nil || string(after) != string(serverBody) {
				t.Fatalf("server unit changed: %q, %v", after, err)
			}
			disabled := false
			for _, call := range runner.calls {
				if call[0] == "start" || call[0] == "stop" {
					t.Fatalf("setup touched runtime state: %v", call)
				}
				if call[0] == "disable" {
					disabled = true
					if call[1] != "tungo-client.service" {
						t.Fatalf("disabled wrong service: %v", call)
					}
				}
			}
			if phase == "enable" && !disabled {
				t.Fatal("cancelled enable left autostart behind")
			}
		})
	}
}

func TestSetup_CleanupDeadlineIsShared(t *testing.T) {
	installer := newSystemdTestInstaller(t, &systemdTestRunner{})
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		installer.runner = systemdRunnerFunc(func(callCtx context.Context, _ string, args ...string) ([]byte, error) {
			if args[0] == "show" {
				return []byte("LoadState=not-found\n"), nil
			}
			if args[0] == "enable" {
				cancel()
				return nil, ctx.Err()
			}
			if ctx.Err() != nil {
				<-callCtx.Done()
				return nil, callCtx.Err()
			}
			return nil, nil
		})
		start := time.Now()
		_, err := installer.Setup(ctx, mode.Client)
		if !errors.Is(err, context.Canceled) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("lost installation or cleanup error: %v", err)
		}
		if time.Since(start) != 2*time.Minute {
			t.Fatalf("cleanup timeout restarted between commands: %v", time.Since(start))
		}
		if _, err := os.Stat(installer.unitPath("tungo-client.service")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("file not cleaned: %v", err)
		}
	})
}

func TestRemoveUnit_RejectsUnmanagedFileBeforeStopping(t *testing.T) {
	runner := &systemdTestRunner{queryResult: map[string]systemdCommandResult{
		"show tungo-client.service": {output: []byte("LoadState=loaded\nExecStart=/opt/tungo c\nFragmentPath=/opt/tungo-client.service\n")},
	}}
	installer := newSystemdTestInstaller(t, runner)
	if err := installer.RemoveUnit(context.Background(), mode.Client); err == nil || !strings.Contains(err.Error(), "not managed") {
		t.Fatalf("unmanaged removal: %v", err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("unmanaged unit was mutated: %v", runner.runs)
	}
}
