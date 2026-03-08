package application

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"tungo/infrastructure/PAL/service_management/linux/systemd/domain"
	"tungo/infrastructure/PAL/service_management/linux/systemd/infrastructure"
)

type mockCommander struct {
	runFn      func(name string, args ...string) error
	combinedFn func(name string, args ...string) ([]byte, error)
}

func (m mockCommander) Run(name string, args ...string) error {
	if m.runFn != nil {
		return m.runFn(name, args...)
	}
	return nil
}

func (m mockCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	if m.combinedFn != nil {
		return m.combinedFn(name, args...)
	}
	return nil, nil
}

type mockFileInfo struct {
	mode os.FileMode
	sys  interface{}
}

func (m mockFileInfo) Name() string       { return "tungo" }
func (m mockFileInfo) Size() int64        { return 1 }
func (m mockFileInfo) Mode() os.FileMode  { return m.mode }
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return m.mode.IsDir() }
func (m mockFileInfo) Sys() interface{}   { return m.sys }

type statUID struct{ Uid uint32 }

func defaultConfig() infrastructure.Config {
	return infrastructure.DefaultConfig()
}

func supportedHooks() infrastructure.Hooks {
	return infrastructure.Hooks{
		Stat:      func(string) (os.FileInfo, error) { return nil, nil },
		LookPath:  func(string) (string, error) { return "/bin/systemctl", nil },
		Lstat:     func(string) (os.FileInfo, error) { return mockFileInfo{mode: 0o755, sys: statUID{Uid: 0}}, nil },
		WriteFile: func(string, []byte, os.FileMode) error { return nil },
		ReadFile:  func(string) ([]byte, error) { return []byte("ExecStart=tungo c\n"), nil },
		Remove:    func(string) error { return nil },
		Geteuid:   func() int { return 0 },
	}
}

func TestServiceSupported(t *testing.T) {
	hooks := supportedHooks()
	s := NewService(mockCommander{}, hooks, defaultConfig())
	if !s.Supported() {
		t.Fatal("expected supported")
	}

	hooks.Stat = func(string) (os.FileInfo, error) { return nil, errors.New("missing") }
	s = NewService(mockCommander{}, hooks, defaultConfig())
	if s.Supported() {
		t.Fatal("expected unsupported when runtime dir missing")
	}

	hooks = supportedHooks()
	hooks.LookPath = func(string) (string, error) { return "", errors.New("missing") }
	s = NewService(mockCommander{}, hooks, defaultConfig())
	if s.Supported() {
		t.Fatal("expected unsupported when systemctl missing")
	}
}

func TestInstallUnit(t *testing.T) {
	cfg := defaultConfig()

	t.Run("unsupported", func(t *testing.T) {
		hooks := supportedHooks()
		hooks.Stat = func(string) (os.FileInfo, error) { return nil, errors.New("missing") }
		s := NewService(mockCommander{}, hooks, cfg)
		if _, err := s.InstallClientUnit(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("not admin", func(t *testing.T) {
		hooks := supportedHooks()
		hooks.Geteuid = func() int { return 1000 }
		s := NewService(mockCommander{}, hooks, cfg)
		if _, err := s.InstallClientUnit(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("binary invalid", func(t *testing.T) {
		hooks := supportedHooks()
		hooks.Lstat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		s := NewService(mockCommander{}, hooks, cfg)
		if _, err := s.InstallClientUnit(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("write failure", func(t *testing.T) {
		hooks := supportedHooks()
		hooks.WriteFile = func(string, []byte, os.FileMode) error { return errors.New("write failed") }
		s := NewService(mockCommander{}, hooks, cfg)
		if _, err := s.InstallClientUnit(); err == nil || !strings.Contains(err.Error(), "failed to write") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("daemon reload failure with rollback", func(t *testing.T) {
		hooks := supportedHooks()
		removed := ""
		hooks.Remove = func(path string) error {
			removed = path
			return nil
		}
		calls := 0
		cmd := mockCommander{runFn: func(_ string, args ...string) error {
			if len(args) > 0 && args[0] == "daemon-reload" {
				calls++
				if calls == 1 {
					return errors.New("reload failed")
				}
			}
			return nil
		}}
		s := NewService(cmd, hooks, cfg)
		if _, err := s.InstallServerUnit(); err == nil || !strings.Contains(err.Error(), "daemon-reload") {
			t.Fatalf("unexpected error: %v", err)
		}
		if removed != cfg.UnitPath {
			t.Fatalf("expected rollback remove %q, got %q", cfg.UnitPath, removed)
		}
	})

	t.Run("enable failure with rollback", func(t *testing.T) {
		hooks := supportedHooks()
		cmd := mockCommander{runFn: func(_ string, args ...string) error {
			if len(args) > 0 && args[0] == "enable" {
				return errors.New("enable failed")
			}
			return nil
		}}
		s := NewService(cmd, hooks, cfg)
		if _, err := s.InstallServerUnit(); err == nil || !strings.Contains(err.Error(), "enable") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		hooks := supportedHooks()
		var wrotePath string
		hooks.WriteFile = func(path string, _ []byte, _ os.FileMode) error {
			wrotePath = path
			return nil
		}
		s := NewService(mockCommander{}, hooks, cfg)
		path, err := s.InstallClientUnit()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != cfg.UnitPath || wrotePath != cfg.UnitPath {
			t.Fatalf("unexpected paths: returned=%q wrote=%q", path, wrotePath)
		}
	})
}

func TestRollbackInstallUnit(t *testing.T) {
	cfg := defaultConfig()

	t.Run("returns original error when rollback clean", func(t *testing.T) {
		hooks := supportedHooks()
		hooks.Remove = func(string) error { return os.ErrNotExist }
		s := NewService(mockCommander{}, hooks, cfg)
		installErr := errors.New("install failed")
		if err := s.rollbackInstallUnit(installErr); !errors.Is(err, installErr) {
			t.Fatalf("expected original error, got %v", err)
		}
	})

	t.Run("joins rollback errors", func(t *testing.T) {
		hooks := supportedHooks()
		hooks.Remove = func(string) error { return errors.New("remove failed") }
		cmd := mockCommander{runFn: func(_ string, args ...string) error {
			if len(args) > 0 && args[0] == "daemon-reload" {
				return errors.New("reload failed")
			}
			return nil
		}}
		s := NewService(cmd, hooks, cfg)
		err := s.rollbackInstallUnit(errors.New("install failed"))
		if err == nil || !strings.Contains(err.Error(), "install failed") || !strings.Contains(err.Error(), "remove failed") {
			t.Fatalf("expected joined error, got %v", err)
		}
	})
}

func TestRemoveUnit(t *testing.T) {
	cfg := defaultConfig()

	t.Run("unsupported", func(t *testing.T) {
		hooks := supportedHooks()
		hooks.Stat = func(string) (os.FileInfo, error) { return nil, errors.New("missing") }
		s := NewService(mockCommander{}, hooks, cfg)
		if err := s.RemoveUnit(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("not admin", func(t *testing.T) {
		hooks := supportedHooks()
		hooks.Geteuid = func() int { return 1000 }
		s := NewService(mockCommander{}, hooks, cfg)
		if err := s.RemoveUnit(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("ignores inactive/disabled/not-exist", func(t *testing.T) {
		hooks := supportedHooks()
		hooks.Remove = func(string) error { return os.ErrNotExist }
		cmd := mockCommander{runFn: func(_ string, args ...string) error {
			switch args[0] {
			case "stop":
				return commandExitError(t, 3)
			case "disable":
				return commandExitError(t, 1)
			default:
				return nil
			}
		}}
		s := NewService(cmd, hooks, cfg)
		if err := s.RemoveUnit(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("stop error", func(t *testing.T) {
		hooks := supportedHooks()
		cmd := mockCommander{runFn: func(_ string, args ...string) error {
			if args[0] == "stop" {
				return errors.New("stop failed")
			}
			return nil
		}}
		s := NewService(cmd, hooks, cfg)
		if err := s.RemoveUnit(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("disable error", func(t *testing.T) {
		hooks := supportedHooks()
		cmd := mockCommander{runFn: func(_ string, args ...string) error {
			if args[0] == "disable" {
				return errors.New("disable failed")
			}
			return nil
		}}
		s := NewService(cmd, hooks, cfg)
		if err := s.RemoveUnit(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("remove error", func(t *testing.T) {
		hooks := supportedHooks()
		hooks.Remove = func(string) error { return errors.New("remove failed") }
		s := NewService(mockCommander{}, hooks, cfg)
		if err := s.RemoveUnit(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("daemon-reload error", func(t *testing.T) {
		hooks := supportedHooks()
		cmd := mockCommander{runFn: func(_ string, args ...string) error {
			if args[0] == "daemon-reload" {
				return errors.New("reload failed")
			}
			return nil
		}}
		s := NewService(cmd, hooks, cfg)
		if err := s.RemoveUnit(); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestIsUnitActive(t *testing.T) {
	cfg := defaultConfig()

	t.Run("unsupported", func(t *testing.T) {
		hooks := supportedHooks()
		hooks.Stat = func(string) (os.FileInfo, error) { return nil, errors.New("missing") }
		s := NewService(mockCommander{}, hooks, cfg)
		if _, err := s.IsUnitActive(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("not active", func(t *testing.T) {
		s := NewService(mockCommander{combinedFn: func(_ string, args ...string) ([]byte, error) {
			if args[0] == "is-active" {
				return nil, commandExitError(t, 3)
			}
			return nil, nil
		}}, supportedHooks(), cfg)
		active, err := s.IsUnitActive()
		if err != nil || active {
			t.Fatalf("expected inactive without error, got active=%v err=%v", active, err)
		}
	})

	t.Run("unexpected error", func(t *testing.T) {
		s := NewService(mockCommander{combinedFn: func(_ string, args ...string) ([]byte, error) {
			if args[0] == "is-active" {
				return nil, errors.New("boom")
			}
			return nil, nil
		}}, supportedHooks(), cfg)
		if _, err := s.IsUnitActive(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("active", func(t *testing.T) {
		s := NewService(mockCommander{combinedFn: func(_ string, args ...string) ([]byte, error) {
			if args[0] == "is-active" {
				return []byte("active\n"), nil
			}
			return nil, nil
		}}, supportedHooks(), cfg)
		active, err := s.IsUnitActive()
		if err != nil || !active {
			t.Fatalf("expected active=true, got active=%v err=%v", active, err)
		}
	})

	t.Run("inactive", func(t *testing.T) {
		s := NewService(mockCommander{combinedFn: func(_ string, args ...string) ([]byte, error) {
			if args[0] == "is-active" {
				return []byte("inactive\n"), nil
			}
			return nil, nil
		}}, supportedHooks(), cfg)
		active, err := s.IsUnitActive()
		if err != nil || active {
			t.Fatalf("expected active=false, got active=%v err=%v", active, err)
		}
	})
}

func TestSimpleUnitOperations(t *testing.T) {
	cfg := defaultConfig()
	ops := []struct {
		name string
		run  func(*Service) error
		cmd  string
	}{
		{"start", (*Service).StartUnit, "start"},
		{"stop", (*Service).StopUnit, "stop"},
		{"enable", (*Service).EnableUnit, "enable"},
		{"disable", (*Service).DisableUnit, "disable"},
	}

	for _, tc := range ops {
		t.Run(tc.name+" unsupported", func(t *testing.T) {
			hooks := supportedHooks()
			hooks.Stat = func(string) (os.FileInfo, error) { return nil, errors.New("missing") }
			s := NewService(mockCommander{}, hooks, cfg)
			if err := tc.run(s); err == nil {
				t.Fatal("expected error")
			}
		})

		t.Run(tc.name+" not-admin", func(t *testing.T) {
			hooks := supportedHooks()
			hooks.Geteuid = func() int { return 1000 }
			s := NewService(mockCommander{}, hooks, cfg)
			if err := tc.run(s); err == nil {
				t.Fatal("expected error")
			}
		})

		t.Run(tc.name+" run-error", func(t *testing.T) {
			cmd := mockCommander{runFn: func(_ string, args ...string) error {
				if args[0] == tc.cmd {
					return errors.New("boom")
				}
				return nil
			}}
			s := NewService(cmd, supportedHooks(), cfg)
			if err := tc.run(s); err == nil {
				t.Fatal("expected error")
			}
		})

		t.Run(tc.name+" success", func(t *testing.T) {
			s := NewService(mockCommander{}, supportedHooks(), cfg)
			if err := tc.run(s); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	cfg := defaultConfig()

	t.Run("unsupported", func(t *testing.T) {
		hooks := supportedHooks()
		hooks.Stat = func(string) (os.FileInfo, error) { return nil, errors.New("missing") }
		s := NewService(mockCommander{}, hooks, cfg)
		if _, err := s.Status(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("is-enabled unexpected error", func(t *testing.T) {
		s := NewService(mockCommander{combinedFn: func(_ string, args ...string) ([]byte, error) {
			if args[0] == "is-enabled" {
				return nil, errors.New("boom")
			}
			return nil, nil
		}}, supportedHooks(), cfg)
		if _, err := s.Status(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("is-active unexpected error", func(t *testing.T) {
		s := NewService(mockCommander{combinedFn: func(_ string, args ...string) ([]byte, error) {
			switch args[0] {
			case "is-enabled":
				return []byte("enabled\n"), nil
			case "is-active":
				return nil, errors.New("boom")
			default:
				return nil, nil
			}
		}}, supportedHooks(), cfg)
		if _, err := s.Status(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("show error", func(t *testing.T) {
		s := NewService(mockCommander{combinedFn: func(_ string, args ...string) ([]byte, error) {
			switch args[0] {
			case "is-enabled":
				return []byte("enabled\n"), nil
			case "is-active":
				return []byte("active\n"), nil
			case "show":
				return nil, errors.New("boom")
			default:
				return nil, nil
			}
		}}, supportedHooks(), cfg)
		if _, err := s.Status(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("not found normalization", func(t *testing.T) {
		s := NewService(mockCommander{combinedFn: func(_ string, args ...string) ([]byte, error) {
			switch args[0] {
			case "is-enabled":
				return []byte("\n"), nil
			case "is-active":
				return []byte("\n"), nil
			case "show":
				return []byte("LoadState=not-found\nSubState=\nResult=\nExecMainStatus=\nExecStart=\nFragmentPath=\n"), nil
			default:
				return nil, nil
			}
		}}, supportedHooks(), cfg)
		status, err := s.Status()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status.Installed {
			t.Fatalf("expected installed=false, got %+v", status)
		}
		if status.UnitFileState != domain.UnitFileStateDisabled || status.ActiveState != domain.UnitActiveStateInactive {
			t.Fatalf("expected disabled/inactive fallback, got %+v", status)
		}
	})

	t.Run("loaded with role from execstart", func(t *testing.T) {
		s := NewService(mockCommander{combinedFn: func(_ string, args ...string) ([]byte, error) {
			switch args[0] {
			case "is-enabled":
				return []byte("enabled\n"), nil
			case "is-active":
				return []byte("active\n"), nil
			case "show":
				return []byte("LoadState=loaded\nActiveState=active\nSubState=running\nResult=success\nExecMainStatus=0\nExecStart=/usr/local/bin/tungo s\nFragmentPath=/etc/systemd/system/tungo.service\n"), nil
			default:
				return nil, nil
			}
		}}, supportedHooks(), cfg)
		status, err := s.Status()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !status.Installed || !status.Managed || status.Role != domain.UnitRoleServer {
			t.Fatalf("unexpected status: %+v", status)
		}
	})

	t.Run("role fallback from unit file", func(t *testing.T) {
		hooks := supportedHooks()
		hooks.ReadFile = func(string) ([]byte, error) { return []byte("ExecStart=tungo c\n"), nil }
		s := NewService(mockCommander{combinedFn: func(_ string, args ...string) ([]byte, error) {
			switch args[0] {
			case "is-enabled":
				return []byte("enabled\n"), nil
			case "is-active":
				return []byte("inactive\n"), nil
			case "show":
				return []byte("LoadState=loaded\nActiveState=inactive\nSubState=dead\nResult=success\nExecMainStatus=0\nExecStart=unknown\nFragmentPath=/usr/lib/systemd/system/tungo.service\n"), nil
			default:
				return nil, nil
			}
		}}, hooks, cfg)
		status, err := s.Status()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status.Role != domain.UnitRoleClient || status.Managed {
			t.Fatalf("unexpected status: %+v", status)
		}
	})

	t.Run("role remains unknown when read fails", func(t *testing.T) {
		hooks := supportedHooks()
		hooks.ReadFile = func(string) ([]byte, error) { return nil, errors.New("read failed") }
		s := NewService(mockCommander{combinedFn: func(_ string, args ...string) ([]byte, error) {
			switch args[0] {
			case "is-enabled":
				return []byte("enabled\n"), nil
			case "is-active":
				return []byte("inactive\n"), nil
			case "show":
				return []byte("LoadState=loaded\nActiveState=inactive\nSubState=dead\nResult=success\nExecMainStatus=0\nExecStart=unknown\nFragmentPath=/usr/lib/systemd/system/tungo.service\n"), nil
			default:
				return nil, nil
			}
		}}, hooks, cfg)
		status, err := s.Status()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status.Role != domain.UnitRoleUnknown {
			t.Fatalf("expected unknown role, got %+v", status)
		}
	})

	t.Run("disabled and inactive errors are accepted", func(t *testing.T) {
		s := NewService(mockCommander{combinedFn: func(_ string, args ...string) ([]byte, error) {
			switch args[0] {
			case "is-enabled":
				return []byte("\n"), commandExitError(t, 1)
			case "is-active":
				return []byte("\n"), commandExitError(t, 3)
			case "show":
				return []byte("LoadState=loaded\nActiveState=inactive\nSubState=dead\nResult=success\nExecMainStatus=0\nExecStart=/usr/local/bin/tungo c\nFragmentPath=/etc/systemd/system/tungo.service\n"), nil
			default:
				return nil, nil
			}
		}}, supportedHooks(), cfg)
		if _, err := s.Status(); err != nil {
			t.Fatalf("expected tolerated errors, got %v", err)
		}
	})
}

func commandExitError(t *testing.T, code int) error {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcessExit")
	cmd.Env = append(
		os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		fmt.Sprintf("GO_HELPER_EXIT_CODE=%d", code),
	)
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit for code %d", code)
	}
	return err
}

func TestHelperProcessExit(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	code, err := strconv.Atoi(os.Getenv("GO_HELPER_EXIT_CODE"))
	if err != nil {
		os.Exit(2)
	}
	os.Exit(code)
}
