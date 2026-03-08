package application

import (
	"errors"
	"fmt"
	"os"
	"tungo/infrastructure/PAL/service_management/linux/systemd/domain"
	"tungo/infrastructure/PAL/service_management/linux/systemd/infrastructure"
)

type Commander interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
	Run(name string, args ...string) error
}

type Config struct {
	RuntimeDir string
	UnitPath   string
	UnitName   string
	BinaryPath string
}

type Service struct {
	commander Commander
	hooks     infrastructure.Hooks
	config    Config
}

func NewService(commander Commander, hooks infrastructure.Hooks, config Config) *Service {
	return &Service{commander: commander, hooks: hooks, config: config}
}

func (s *Service) Supported() bool {
	return infrastructure.Supported(s.hooks, s.config.RuntimeDir)
}

func (s *Service) InstallServerUnit() (string, error) {
	return s.installUnit("s")
}

func (s *Service) InstallClientUnit() (string, error) {
	return s.installUnit("c")
}

func (s *Service) installUnit(modeArg string) (string, error) {
	if !s.Supported() {
		return "", fmt.Errorf("systemd is not supported on this platform")
	}
	if err := infrastructure.RequireAdminPrivileges(s.hooks); err != nil {
		return "", err
	}
	if err := infrastructure.ValidateTungoBinaryForSystemd(s.hooks, s.config.BinaryPath); err != nil {
		return "", err
	}
	if err := s.hooks.WriteFile(s.config.UnitPath, []byte(infrastructure.UnitFileContent(s.config.BinaryPath, modeArg)), 0644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", s.config.UnitPath, err)
	}
	if err := s.commander.Run("systemctl", "daemon-reload"); err != nil {
		return "", s.rollbackInstallUnit(fmt.Errorf("failed to run systemctl daemon-reload: %w", err))
	}
	if err := s.commander.Run("systemctl", "enable", s.config.UnitName); err != nil {
		return "", s.rollbackInstallUnit(fmt.Errorf("failed to run systemctl enable %s: %w", s.config.UnitName, err))
	}
	return s.config.UnitPath, nil
}

func (s *Service) rollbackInstallUnit(installErr error) error {
	rollbackErrs := make([]error, 0, 2)
	if err := s.hooks.Remove(s.config.UnitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		rollbackErrs = append(rollbackErrs, fmt.Errorf("failed to rollback %s: %w", s.config.UnitPath, err))
	}
	if err := s.commander.Run("systemctl", "daemon-reload"); err != nil {
		rollbackErrs = append(rollbackErrs, fmt.Errorf("failed to rollback systemctl daemon-reload: %w", err))
	}
	if len(rollbackErrs) == 0 {
		return installErr
	}
	rollbackErrs = append([]error{installErr}, rollbackErrs...)
	return errors.Join(rollbackErrs...)
}

func (s *Service) RemoveUnit() error {
	if !s.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}
	if err := infrastructure.RequireAdminPrivileges(s.hooks); err != nil {
		return err
	}

	if err := s.commander.Run("systemctl", "stop", s.config.UnitName); err != nil && !infrastructure.IsSystemdNotActiveError(err) {
		return fmt.Errorf("failed to run systemctl stop %s: %w", s.config.UnitName, err)
	}
	if err := s.commander.Run("systemctl", "disable", s.config.UnitName); err != nil && !infrastructure.IsSystemdDisabledError(err) {
		return fmt.Errorf("failed to run systemctl disable %s: %w", s.config.UnitName, err)
	}
	if err := s.hooks.Remove(s.config.UnitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove %s: %w", s.config.UnitPath, err)
	}
	if err := s.commander.Run("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("failed to run systemctl daemon-reload: %w", err)
	}
	return nil
}

func (s *Service) IsUnitActive() (bool, error) {
	if !s.Supported() {
		return false, fmt.Errorf("systemd is not supported on this platform")
	}
	activeOutput, err := s.commander.CombinedOutput("systemctl", "is-active", s.config.UnitName)
	if err != nil {
		if infrastructure.IsSystemdNotActiveError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to run systemctl is-active %s: %w", s.config.UnitName, err)
	}
	return domain.ActiveStateBlocksRuntimeStart(infrastructure.ParseUnitActiveState(activeOutput, nil)), nil
}

func (s *Service) StopUnit() error {
	if !s.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}
	if err := infrastructure.RequireAdminPrivileges(s.hooks); err != nil {
		return err
	}
	if err := s.commander.Run("systemctl", "stop", s.config.UnitName); err != nil {
		return fmt.Errorf("failed to run systemctl stop %s: %w", s.config.UnitName, err)
	}
	return nil
}

func (s *Service) StartUnit() error {
	if !s.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}
	if err := infrastructure.RequireAdminPrivileges(s.hooks); err != nil {
		return err
	}
	if err := s.commander.Run("systemctl", "start", s.config.UnitName); err != nil {
		return fmt.Errorf("failed to run systemctl start %s: %w", s.config.UnitName, err)
	}
	return nil
}

func (s *Service) EnableUnit() error {
	if !s.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}
	if err := infrastructure.RequireAdminPrivileges(s.hooks); err != nil {
		return err
	}
	if err := s.commander.Run("systemctl", "enable", s.config.UnitName); err != nil {
		return fmt.Errorf("failed to run systemctl enable %s: %w", s.config.UnitName, err)
	}
	return nil
}

func (s *Service) DisableUnit() error {
	if !s.Supported() {
		return fmt.Errorf("systemd is not supported on this platform")
	}
	if err := infrastructure.RequireAdminPrivileges(s.hooks); err != nil {
		return err
	}
	if err := s.commander.Run("systemctl", "disable", s.config.UnitName); err != nil {
		return fmt.Errorf("failed to run systemctl disable %s: %w", s.config.UnitName, err)
	}
	return nil
}

func (s *Service) Status() (domain.UnitStatus, error) {
	if !s.Supported() {
		return domain.UnitStatus{}, fmt.Errorf("systemd is not supported on this platform")
	}

	status := domain.UnitStatus{
		Role:           domain.UnitRoleUnknown,
		LoadState:      domain.UnitLoadStateUnknown,
		UnitFileState:  domain.UnitFileStateUnknown,
		ActiveState:    domain.UnitActiveStateUnknown,
		SubState:       "unknown",
		Result:         "unknown",
		ExecMainStatus: "unknown",
		ExecStart:      "unknown",
		FragmentPath:   "unknown",
	}

	enabledOutput, err := s.commander.CombinedOutput("systemctl", "is-enabled", s.config.UnitName)
	if err != nil {
		if !infrastructure.IsSystemdDisabledError(err) {
			return domain.UnitStatus{}, fmt.Errorf("failed to run systemctl is-enabled %s: %w", s.config.UnitName, err)
		}
	}
	status.UnitFileState = infrastructure.ParseUnitFileState(enabledOutput, err)

	activeOutput, activeErr := s.commander.CombinedOutput("systemctl", "is-active", s.config.UnitName)
	if activeErr != nil {
		if !infrastructure.IsSystemdNotActiveError(activeErr) {
			return domain.UnitStatus{}, fmt.Errorf("failed to run systemctl is-active %s: %w", s.config.UnitName, activeErr)
		}
	}
	status.ActiveState = infrastructure.ParseUnitActiveState(activeOutput, activeErr)

	showOutput, showErr := s.commander.CombinedOutput(
		"systemctl",
		"show",
		s.config.UnitName,
		"--property=LoadState,ActiveState,SubState,Result,ExecMainStatus,ExecStart,FragmentPath",
		"--no-page",
	)
	if showErr != nil {
		return domain.UnitStatus{}, fmt.Errorf("failed to run systemctl show %s: %w", s.config.UnitName, showErr)
	}
	props := infrastructure.ParseSystemdShowProperties(showOutput)
	status.LoadState = domain.UnitLoadState(infrastructure.NormalizeSystemdValue(props["LoadState"]))
	if activeFromShow := domain.UnitActiveState(infrastructure.NormalizeSystemdValue(props["ActiveState"])); activeFromShow != domain.UnitActiveStateUnknown {
		status.ActiveState = activeFromShow
	}
	status.SubState = infrastructure.NormalizeSystemdValue(props["SubState"])
	status.Result = infrastructure.NormalizeSystemdValue(props["Result"])
	status.ExecMainStatus = infrastructure.NormalizeSystemdValue(props["ExecMainStatus"])
	status.ExecStart = infrastructure.NormalizeSystemdRawValue(props["ExecStart"])
	status.FragmentPath = infrastructure.NormalizeSystemdRawValue(props["FragmentPath"])
	status.Managed = domain.IsInstallerManagedFragmentPath(status.FragmentPath, s.config.UnitPath)

	switch status.LoadState {
	case domain.UnitLoadStateNotFound:
		status.Installed = false
		if status.UnitFileState == domain.UnitFileStateUnknown {
			status.UnitFileState = domain.UnitFileStateDisabled
		}
		if status.ActiveState == domain.UnitActiveStateUnknown {
			status.ActiveState = domain.UnitActiveStateInactive
		}
	default:
		status.Installed = status.LoadState != domain.UnitLoadStateUnknown ||
			status.UnitFileState != domain.UnitFileStateUnknown ||
			status.ActiveState != domain.UnitActiveStateUnknown
	}

	status.Role = domain.DetectUnitRoleFromExecStart(status.ExecStart)
	if status.Role == domain.UnitRoleUnknown {
		if unitBody, readErr := s.hooks.ReadFile(s.config.UnitPath); readErr == nil {
			status.Role = domain.DetectUnitRole(string(unitBody))
		}
	}
	return status, nil
}
