package systemd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"tungo/infrastructure/PAL/exec_commander"
)

const (
	systemdRuntimeDir = "/run/systemd/system"
	systemdUnitPath   = "/etc/systemd/system/tungo.service"
	systemdUnitName   = "tungo.service"
	tungoBinaryPath   = "/usr/local/bin/tungo"
)

var (
	statPath      = os.Stat
	lookPath      = exec.LookPath
	writeFilePath = os.WriteFile
	readFilePath  = os.ReadFile
	removePath    = os.Remove
	geteuid       = os.Geteuid
)

type UnitRole string

const (
	UnitRoleUnknown UnitRole = "unknown"
	UnitRoleClient  UnitRole = "client"
	UnitRoleServer  UnitRole = "server"
)

type UnitStatus struct {
	Installed bool
	Enabled   bool
	Active    bool
	Role      UnitRole
}

type Installer interface {
	Supported() bool
	InstallServerUnit() (string, error)
	InstallClientUnit() (string, error)
	RemoveUnit() error
	IsUnitActive() (bool, error)
	StopUnit() error
	StartUnit() error
	EnableUnit() error
	DisableUnit() error
	Status() (UnitStatus, error)
}

type UnitInstaller struct {
	commander exec_commander.Commander
}

func NewUnitInstaller(commander exec_commander.Commander) Installer {
	if commander == nil {
		commander = exec_commander.NewExecCommander()
	}
	return &UnitInstaller{commander: commander}
}

func (i *UnitInstaller) Supported() bool {
	if _, err := statPath(systemdRuntimeDir); err != nil {
		return false
	}
	if _, err := lookPath("systemctl"); err != nil {
		return false
	}
	return true
}

func (i *UnitInstaller) InstallServerUnit() (string, error) {
	return i.installUnit("s")
}

func (i *UnitInstaller) InstallClientUnit() (string, error) {
	return i.installUnit("c")
}

func (i *UnitInstaller) RemoveUnit() error {
	if !i.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}
	if err := requireAdminPrivileges(); err != nil {
		return err
	}

	if err := i.commander.Run("systemctl", "stop", systemdUnitName); err != nil && !isSystemdNotActiveError(err) {
		return fmt.Errorf("failed to run systemctl stop %s: %w", systemdUnitName, err)
	}
	if err := i.commander.Run("systemctl", "disable", systemdUnitName); err != nil && !isSystemdDisabledError(err) {
		return fmt.Errorf("failed to run systemctl disable %s: %w", systemdUnitName, err)
	}
	if err := removePath(systemdUnitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove %s: %w", systemdUnitPath, err)
	}
	if err := i.commander.Run("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("failed to run systemctl daemon-reload: %w", err)
	}
	return nil
}

func (i *UnitInstaller) installUnit(modeArg string) (string, error) {
	if !i.Supported() {
		return "", fmt.Errorf("systemd is not supported on this platform")
	}
	if err := requireAdminPrivileges(); err != nil {
		return "", err
	}
	if err := validateTungoBinaryForSystemd(); err != nil {
		return "", err
	}
	if err := writeFilePath(systemdUnitPath, []byte(unitFileContent(modeArg)), 0644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", systemdUnitPath, err)
	}
	if err := i.commander.Run("systemctl", "daemon-reload"); err != nil {
		return "", fmt.Errorf("failed to run systemctl daemon-reload: %w", err)
	}
	if err := i.commander.Run("systemctl", "enable", systemdUnitName); err != nil {
		return "", fmt.Errorf("failed to run systemctl enable %s: %w", systemdUnitName, err)
	}
	return systemdUnitPath, nil
}

func (i *UnitInstaller) IsUnitActive() (bool, error) {
	if !i.Supported() {
		return false, fmt.Errorf("systemd is not supported on this platform")
	}
	if err := i.commander.Run("systemctl", "is-active", "--quiet", systemdUnitName); err != nil {
		if isSystemdNotActiveError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to run systemctl is-active %s: %w", systemdUnitName, err)
	}
	return true, nil
}

func (i *UnitInstaller) StopUnit() error {
	if !i.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}
	if err := requireAdminPrivileges(); err != nil {
		return err
	}
	if err := i.commander.Run("systemctl", "stop", systemdUnitName); err != nil {
		return fmt.Errorf("failed to run systemctl stop %s: %w", systemdUnitName, err)
	}
	return nil
}

func (i *UnitInstaller) StartUnit() error {
	if !i.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}
	if err := requireAdminPrivileges(); err != nil {
		return err
	}
	if err := i.commander.Run("systemctl", "start", systemdUnitName); err != nil {
		return fmt.Errorf("failed to run systemctl start %s: %w", systemdUnitName, err)
	}
	return nil
}

func (i *UnitInstaller) EnableUnit() error {
	if !i.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}
	if err := requireAdminPrivileges(); err != nil {
		return err
	}
	if err := i.commander.Run("systemctl", "enable", systemdUnitName); err != nil {
		return fmt.Errorf("failed to run systemctl enable %s: %w", systemdUnitName, err)
	}
	return nil
}

func (i *UnitInstaller) DisableUnit() error {
	if !i.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}
	if err := requireAdminPrivileges(); err != nil {
		return err
	}
	if err := i.commander.Run("systemctl", "disable", systemdUnitName); err != nil {
		return fmt.Errorf("failed to run systemctl disable %s: %w", systemdUnitName, err)
	}
	return nil
}

func (i *UnitInstaller) Status() (UnitStatus, error) {
	if !i.Supported() {
		return UnitStatus{}, fmt.Errorf("systemd is not supported on this platform")
	}

	status := UnitStatus{
		Role: UnitRoleUnknown,
	}
	if _, err := statPath(systemdUnitPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return status, nil
		}
		return UnitStatus{}, fmt.Errorf("failed to stat %s: %w", systemdUnitPath, err)
	}
	status.Installed = true

	enabledOutput, err := i.commander.CombinedOutput("systemctl", "is-enabled", systemdUnitName)
	if err != nil {
		if !isSystemdDisabledError(err) {
			return UnitStatus{}, fmt.Errorf("failed to run systemctl is-enabled %s: %w", systemdUnitName, err)
		}
	}
	status.Enabled = strings.EqualFold(strings.TrimSpace(string(enabledOutput)), "enabled")

	active, err := i.IsUnitActive()
	if err != nil {
		return UnitStatus{}, err
	}
	status.Active = active

	unitBody, err := readFilePath(systemdUnitPath)
	if err != nil {
		return UnitStatus{}, fmt.Errorf("failed to read %s: %w", systemdUnitPath, err)
	}
	status.Role = detectUnitRole(string(unitBody))
	return status, nil
}

func isSystemdNotActiveError(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	code := exitErr.ExitCode()
	return code == 3 || code == 4
}

func isSystemdDisabledError(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	code := exitErr.ExitCode()
	return code == 1 || code == 3 || code == 4
}

func detectUnitRole(unitBody string) UnitRole {
	for _, line := range strings.Split(unitBody, "\n") {
		if !strings.HasPrefix(line, "ExecStart=") {
			continue
		}
		command := strings.TrimSpace(strings.TrimPrefix(line, "ExecStart="))
		command = strings.TrimPrefix(command, "-")
		fields := strings.Fields(command)
		if len(fields) == 0 {
			continue
		}
		for i := 0; i < len(fields); i++ {
			if filepath.Base(fields[i]) != "tungo" {
				continue
			}
			for j := i + 1; j < len(fields); j++ {
				switch fields[j] {
				case "c":
					return UnitRoleClient
				case "s":
					return UnitRoleServer
				}
			}
		}
	}
	return UnitRoleUnknown
}

func unitFileContent(modeArg string) string {
	return fmt.Sprintf(`[Unit]
Description=TunGo VPN Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s %s
User=root
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, tungoBinaryPath, modeArg)
}

func requireAdminPrivileges() error {
	if geteuid() == 0 {
		return nil
	}
	return errors.New("admin privileges are required to manage tungo systemd service")
}

func validateTungoBinaryForSystemd() error {
	info, err := statPath(tungoBinaryPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("tungo executable is not installed at /usr/local/bin/tungo; install it using the official Linux guide")
		}
		return fmt.Errorf("failed to stat %s: %w", tungoBinaryPath, err)
	}
	if info == nil {
		return fmt.Errorf("failed to stat %s: empty file info", tungoBinaryPath)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", tungoBinaryPath)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("%s is not executable", tungoBinaryPath)
	}
	if info.Mode()&0o022 != 0 {
		return fmt.Errorf("%s must not be writable by group or others", tungoBinaryPath)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("failed to verify owner of %s", tungoBinaryPath)
	}
	if stat.Uid != 0 {
		return fmt.Errorf("%s must be owned by root", tungoBinaryPath)
	}
	return nil
}
