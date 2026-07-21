package systemd

import (
	"errors"
	"fmt"
	"os"

	"tungo/application/commandline"
	"tungo/application/runtime"
	"tungo/infrastructure/PAL/exec_commander"
)

type UnitInstaller struct {
	commander exec_commander.Commander
	hooks     Hooks
	config    Config
}

func NewUnitInstaller(commander exec_commander.Commander) *UnitInstaller {
	if commander == nil {
		commander = exec_commander.NewExecCommander()
	}
	return &UnitInstaller{
		commander: commander,
		hooks: Hooks{
			Stat:      statPath,
			Lstat:     lstatPath,
			LookPath:  lookPath,
			WriteFile: writeFilePath,
			ReadFile:  readFilePath,
			Remove:    removePath,
		},
		config: defaultSystemdConfig,
	}
}

func (i *UnitInstaller) Supported() bool {
	return Supported(i.hooks, i.config.RuntimeDir)
}

func (i *UnitInstaller) InstallServerUnit() (string, error) {
	return i.installRuntimeUnit(runtime.ModeServer)
}

func (i *UnitInstaller) InstallClientUnit() (string, error) {
	return i.installRuntimeUnit(runtime.ModeClient)
}

func (i *UnitInstaller) installRuntimeUnit(mode runtime.Mode) (string, error) {
	args, err := commandline.RuntimeModeArgs(mode)
	if err != nil {
		return "", err
	}
	return i.installUnit(args)
}

func (i *UnitInstaller) installUnit(args []string) (string, error) {
	if !i.Supported() {
		return "", fmt.Errorf("systemd is not supported on this platform")
	}
	if err := ValidateTungoBinaryForSystemd(i.hooks, i.config.BinaryPath); err != nil {
		return "", err
	}
	if err := i.hooks.WriteFile(i.config.UnitPath, []byte(UnitFileContent(i.config.BinaryPath, args)), 0644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", i.config.UnitPath, err)
	}
	if err := i.commander.Run("systemctl", "daemon-reload"); err != nil {
		return "", i.rollbackInstallUnit(fmt.Errorf("failed to run systemctl daemon-reload: %w", err))
	}
	if err := i.commander.Run("systemctl", "enable", i.config.UnitName); err != nil {
		return "", i.rollbackInstallUnit(fmt.Errorf("failed to run systemctl enable %s: %w", i.config.UnitName, err))
	}
	return i.config.UnitPath, nil
}

func (i *UnitInstaller) rollbackInstallUnit(installErr error) error {
	rollbackErrs := make([]error, 0, 2)
	if err := i.hooks.Remove(i.config.UnitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		rollbackErrs = append(rollbackErrs, fmt.Errorf("failed to rollback %s: %w", i.config.UnitPath, err))
	}
	if err := i.commander.Run("systemctl", "daemon-reload"); err != nil {
		rollbackErrs = append(rollbackErrs, fmt.Errorf("failed to rollback systemctl daemon-reload: %w", err))
	}
	if len(rollbackErrs) == 0 {
		return installErr
	}
	rollbackErrs = append([]error{installErr}, rollbackErrs...)
	return errors.Join(rollbackErrs...)
}

func (i *UnitInstaller) RemoveUnit() error {
	if !i.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}

	if err := i.commander.Run("systemctl", "stop", i.config.UnitName); err != nil && !IsSystemdNotActiveError(err) {
		return fmt.Errorf("failed to run systemctl stop %s: %w", i.config.UnitName, err)
	}
	if err := i.commander.Run("systemctl", "disable", i.config.UnitName); err != nil && !IsSystemdDisabledError(err) {
		return fmt.Errorf("failed to run systemctl disable %s: %w", i.config.UnitName, err)
	}
	if err := i.hooks.Remove(i.config.UnitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove %s: %w", i.config.UnitPath, err)
	}
	if err := i.commander.Run("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("failed to run systemctl daemon-reload: %w", err)
	}
	return nil
}

func (i *UnitInstaller) IsUnitActive() (bool, error) {
	if !i.Supported() {
		return false, fmt.Errorf("systemd is not supported on this platform")
	}
	activeOutput, err := i.commander.CombinedOutput("systemctl", "is-active", i.config.UnitName)
	if err != nil {
		if IsSystemdNotActiveError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to run systemctl is-active %s: %w", i.config.UnitName, err)
	}
	return ActiveStateBlocksRuntimeStart(ParseUnitActiveState(activeOutput, nil)), nil
}

func (i *UnitInstaller) StopUnit() error {
	if !i.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}
	if err := i.commander.Run("systemctl", "stop", i.config.UnitName); err != nil {
		return fmt.Errorf("failed to run systemctl stop %s: %w", i.config.UnitName, err)
	}
	return nil
}

func (i *UnitInstaller) StartUnit() error {
	if !i.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}
	if err := i.commander.Run("systemctl", "start", i.config.UnitName); err != nil {
		return fmt.Errorf("failed to run systemctl start %s: %w", i.config.UnitName, err)
	}
	return nil
}

func (i *UnitInstaller) EnableUnit() error {
	if !i.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}
	if err := i.commander.Run("systemctl", "enable", i.config.UnitName); err != nil {
		return fmt.Errorf("failed to run systemctl enable %s: %w", i.config.UnitName, err)
	}
	return nil
}

func (i *UnitInstaller) DisableUnit() error {
	if !i.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}
	if err := i.commander.Run("systemctl", "disable", i.config.UnitName); err != nil {
		return fmt.Errorf("failed to run systemctl disable %s: %w", i.config.UnitName, err)
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
		ExecStart:      "unknown",
		FragmentPath:   "unknown",
	}

	enabledOutput, err := i.commander.CombinedOutput("systemctl", "is-enabled", i.config.UnitName)
	if err != nil {
		if !IsSystemdDisabledError(err) {
			return UnitStatus{}, fmt.Errorf("failed to run systemctl is-enabled %s: %w", i.config.UnitName, err)
		}
	}
	status.UnitFileState = ParseUnitFileState(enabledOutput, err)

	activeOutput, activeErr := i.commander.CombinedOutput("systemctl", "is-active", i.config.UnitName)
	if activeErr != nil {
		if !IsSystemdNotActiveError(activeErr) {
			return UnitStatus{}, fmt.Errorf("failed to run systemctl is-active %s: %w", i.config.UnitName, activeErr)
		}
	}
	status.ActiveState = ParseUnitActiveState(activeOutput, activeErr)

	showOutput, showErr := i.commander.CombinedOutput(
		"systemctl",
		"show",
		i.config.UnitName,
		"--property=LoadState,ActiveState,SubState,Result,ExecMainStatus,ExecStart,FragmentPath",
		"--no-page",
	)
	if showErr != nil {
		return UnitStatus{}, fmt.Errorf("failed to run systemctl show %s: %w", i.config.UnitName, showErr)
	}
	props := ParseSystemdShowProperties(showOutput)
	status.LoadState = UnitLoadState(NormalizeSystemdValue(props["LoadState"]))
	if activeFromShow := UnitActiveState(NormalizeSystemdValue(props["ActiveState"])); activeFromShow != UnitActiveStateUnknown {
		status.ActiveState = activeFromShow
	}
	status.SubState = NormalizeSystemdValue(props["SubState"])
	status.Result = NormalizeSystemdValue(props["Result"])
	status.ExecMainStatus = NormalizeSystemdValue(props["ExecMainStatus"])
	status.ExecStart = NormalizeSystemdRawValue(props["ExecStart"])
	status.FragmentPath = NormalizeSystemdRawValue(props["FragmentPath"])
	status.Managed = IsInstallerManagedFragmentPath(status.FragmentPath, i.config.UnitPath)

	switch status.LoadState {
	case UnitLoadStateNotFound:
		status.Installed = false
		if status.UnitFileState == UnitFileStateUnknown {
			status.UnitFileState = UnitFileStateDisabled
		}
		if status.ActiveState == UnitActiveStateUnknown {
			status.ActiveState = UnitActiveStateInactive
		}
	default:
		status.Installed = status.LoadState != UnitLoadStateUnknown ||
			status.UnitFileState != UnitFileStateUnknown ||
			status.ActiveState != UnitActiveStateUnknown
	}

	status.Role = DetectUnitRoleFromExecStart(status.ExecStart)
	if status.Role == UnitRoleUnknown && status.Managed {
		if unitBody, readErr := i.hooks.ReadFile(i.config.UnitPath); readErr == nil {
			status.Role = DetectUnitRole(string(unitBody))
		}
	}
	return status, nil
}
