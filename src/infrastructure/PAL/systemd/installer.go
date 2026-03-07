package systemd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
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
	lstatPath     = os.Lstat
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

type UnitLoadState string

const (
	UnitLoadStateUnknown  UnitLoadState = "unknown"
	UnitLoadStateLoaded   UnitLoadState = "loaded"
	UnitLoadStateNotFound UnitLoadState = "not-found"
)

type UnitFileState string

const (
	UnitFileStateUnknown  UnitFileState = "unknown"
	UnitFileStateEnabled  UnitFileState = "enabled"
	UnitFileStateDisabled UnitFileState = "disabled"
)

type UnitActiveState string

const (
	UnitActiveStateUnknown      UnitActiveState = "unknown"
	UnitActiveStateActive       UnitActiveState = "active"
	UnitActiveStateReloading    UnitActiveState = "reloading"
	UnitActiveStateInactive     UnitActiveState = "inactive"
	UnitActiveStateFailed       UnitActiveState = "failed"
	UnitActiveStateActivating   UnitActiveState = "activating"
	UnitActiveStateDeactivating UnitActiveState = "deactivating"
)

type UnitStatus struct {
	Installed      bool
	Role           UnitRole
	LoadState      UnitLoadState
	UnitFileState  UnitFileState
	ActiveState    UnitActiveState
	SubState       string
	Result         string
	ExecMainStatus string
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
	activeOutput, err := i.commander.CombinedOutput("systemctl", "is-active", systemdUnitName)
	if err != nil {
		if isSystemdNotActiveError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to run systemctl is-active %s: %w", systemdUnitName, err)
	}
	return activeStateIndicatesRunning(parseUnitActiveState(activeOutput, nil)), nil
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
		Role:           UnitRoleUnknown,
		LoadState:      UnitLoadStateUnknown,
		UnitFileState:  UnitFileStateUnknown,
		ActiveState:    UnitActiveStateUnknown,
		SubState:       "unknown",
		Result:         "unknown",
		ExecMainStatus: "unknown",
	}
	if _, err := statPath(systemdUnitPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			status.LoadState = UnitLoadStateNotFound
			status.UnitFileState = UnitFileStateDisabled
			status.ActiveState = UnitActiveStateInactive
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
	status.UnitFileState = parseUnitFileState(enabledOutput, err)

	activeOutput, activeErr := i.commander.CombinedOutput("systemctl", "is-active", systemdUnitName)
	if activeErr != nil {
		if !isSystemdNotActiveError(activeErr) {
			return UnitStatus{}, fmt.Errorf("failed to run systemctl is-active %s: %w", systemdUnitName, activeErr)
		}
	}
	status.ActiveState = parseUnitActiveState(activeOutput, activeErr)

	showOutput, showErr := i.commander.CombinedOutput(
		"systemctl",
		"show",
		systemdUnitName,
		"--property=LoadState,SubState,Result,ExecMainStatus",
		"--no-page",
	)
	if showErr != nil {
		return UnitStatus{}, fmt.Errorf("failed to run systemctl show %s: %w", systemdUnitName, showErr)
	}
	props := parseSystemdShowProperties(showOutput)
	status.LoadState = UnitLoadState(normalizeSystemdValue(props["LoadState"]))
	status.SubState = normalizeSystemdValue(props["SubState"])
	status.Result = normalizeSystemdValue(props["Result"])
	status.ExecMainStatus = normalizeSystemdValue(props["ExecMainStatus"])

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

func ActiveStateBlocksRuntimeStart(state UnitActiveState) bool {
	switch state {
	case UnitActiveStateActive, UnitActiveStateReloading, UnitActiveStateActivating, UnitActiveStateDeactivating:
		return true
	default:
		return false
	}
}

func activeStateIndicatesRunning(state UnitActiveState) bool {
	switch state {
	case UnitActiveStateActive, UnitActiveStateReloading:
		return true
	default:
		return false
	}
}

func parseUnitFileState(output []byte, err error) UnitFileState {
	state := UnitFileState(normalizeSystemdValue(string(output)))
	if state == UnitFileStateUnknown && err != nil && isSystemdDisabledError(err) {
		return UnitFileStateDisabled
	}
	return state
}

func parseUnitActiveState(output []byte, err error) UnitActiveState {
	state := UnitActiveState(normalizeSystemdValue(string(output)))
	if state == UnitActiveStateUnknown && err != nil && isSystemdNotActiveError(err) {
		return UnitActiveStateInactive
	}
	return state
}

func parseSystemdShowProperties(output []byte) map[string]string {
	properties := make(map[string]string, 4)
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		properties[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return properties
}

func normalizeSystemdValue(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "unknown"
	}
	return normalized
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
	info, err := lstatPath(tungoBinaryPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("tungo executable is not installed at /usr/local/bin/tungo; install it using the official Linux guide")
		}
		return fmt.Errorf("failed to lstat %s: %w", tungoBinaryPath, err)
	}
	if info == nil {
		return fmt.Errorf("failed to lstat %s: empty file info", tungoBinaryPath)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symlink", tungoBinaryPath)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", tungoBinaryPath)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("%s is not executable", tungoBinaryPath)
	}
	if info.Mode()&0o022 != 0 {
		return fmt.Errorf("%s must not be writable by group or others; run: sudo chmod 0755 %s", tungoBinaryPath, tungoBinaryPath)
	}
	uid, ok := fileOwnerUID(info)
	if !ok {
		return fmt.Errorf("failed to verify owner of %s", tungoBinaryPath)
	}
	if uid != 0 {
		return fmt.Errorf("%s must be owned by root; run: sudo chown root:root %s", tungoBinaryPath, tungoBinaryPath)
	}
	return nil
}

func fileOwnerUID(info os.FileInfo) (uint64, bool) {
	sys := info.Sys()
	if sys == nil {
		return 0, false
	}

	v := reflect.ValueOf(sys)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return 0, false
		}
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return 0, false
	}

	uidField := v.FieldByName("Uid")
	if !uidField.IsValid() {
		return 0, false
	}

	switch uidField.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uidField.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		uid := uidField.Int()
		if uid < 0 {
			return 0, false
		}
		return uint64(uid), true
	default:
		return 0, false
	}
}
