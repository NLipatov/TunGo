package systemd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"tungo/infrastructure/PAL/exec_commander"
)

const (
	systemdRuntimeDir = "/run/systemd/system"
	systemdUnitPath   = "/etc/systemd/system/tungo.service"
	systemdUnitName   = "tungo.service"
)

var (
	statPath      = os.Stat
	lookPath      = exec.LookPath
	writeFilePath = os.WriteFile
)

type Installer interface {
	Supported() bool
	InstallServerUnit() (string, error)
	InstallClientUnit() (string, error)
	IsUnitActive() (bool, error)
	StopUnit() error
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

func (i *UnitInstaller) installUnit(modeArg string) (string, error) {
	if !i.Supported() {
		return "", fmt.Errorf("systemd is not supported on this platform")
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
	if err := i.commander.Run("systemctl", "stop", systemdUnitName); err != nil {
		return fmt.Errorf("failed to run systemctl stop %s: %w", systemdUnitName, err)
	}
	return nil
}

func isSystemdNotActiveError(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	code := exitErr.ExitCode()
	return code == 3 || code == 4
}

func unitFileContent(modeArg string) string {
	return fmt.Sprintf(`[Unit]
Description=TunGo VPN Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=tungo %s
User=root
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, modeArg)
}
