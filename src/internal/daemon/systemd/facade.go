package systemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"tungo/internal/commandline"
	"tungo/internal/mode"
)

// Control exposes systemd unit management operations.
type Control interface {
	Setup(context.Context, mode.Mode) (string, error)
	RemoveUnit(context.Context) error
	IsUnitActive(context.Context) (bool, error)
	StopUnit(context.Context) error
	StartUnit(context.Context) error
	EnableUnit(context.Context) error
	DisableUnit(context.Context) error
	Status(context.Context) (UnitStatus, error)
}

type runner interface {
	CombinedOutputContext(ctx context.Context, name string, args ...string) ([]byte, error)
}

type UnitInstaller struct {
	runner runner
	config Config
}

func NewUnitInstaller(runner runner) *UnitInstaller {
	return &UnitInstaller{
		runner: runner,
		config: DefaultConfig(),
	}
}

func (i *UnitInstaller) Available() bool {
	return available(i.config.RuntimeDir)
}

// Setup creates a new unit. Existing services must be explicitly removed first.
func (i *UnitInstaller) Setup(ctx context.Context, runtimeMode mode.Mode) (path string, err error) {
	args, err := commandline.RuntimeModeArgs(runtimeMode)
	if err != nil {
		return "", fmt.Errorf("invalid daemon mode: %v", runtimeMode)
	}
	if err := validateTungoBinaryForSystemd(i.config.BinaryPath); err != nil {
		return "", err
	}
	status, err := i.Status(ctx)
	if err != nil {
		return "", err
	}
	if status.Installed {
		return "", fmt.Errorf("%s is already installed; remove it before installing again", i.config.UnitName)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path = i.config.UnitPath
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create %s: %w", path, err)
	}
	// Cleanup only what this installation created, even when the TUI exits.
	unitPath := path
	enableAttempted := false
	defer func() {
		if err == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		if enableAttempted {
			_, disableErr := i.run(cleanupCtx, "disable", i.config.UnitName)
			err = errors.Join(err, disableErr)
		}
		removeErr := os.Remove(unitPath)
		_, reloadErr := i.run(cleanupCtx, "daemon-reload")
		err = errors.Join(err, removeErr, reloadErr)
	}()
	_, writeErr := file.WriteString(UnitFileContent(i.config.BinaryPath, args))
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", path, err)
	}
	if _, err := i.run(ctx, "daemon-reload"); err != nil {
		return "", err
	}
	enableAttempted = true
	if _, err := i.run(ctx, "enable", i.config.UnitName); err != nil {
		return "", err
	}
	return path, nil
}

func (i *UnitInstaller) RemoveUnit(ctx context.Context) error {
	if !i.Available() {
		return fmt.Errorf("systemd is not available")
	}

	if _, err := i.run(ctx, "stop", i.config.UnitName); err != nil && !IsSystemdNotActiveError(err) {
		return fmt.Errorf("failed to run systemctl stop %s: %w", i.config.UnitName, err)
	}
	if _, err := i.run(ctx, "disable", i.config.UnitName); err != nil && !IsSystemdDisabledError(err) {
		return fmt.Errorf("failed to run systemctl disable %s: %w", i.config.UnitName, err)
	}
	if err := os.Remove(i.config.UnitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove %s: %w", i.config.UnitPath, err)
	}
	if _, err := i.run(ctx, "daemon-reload"); err != nil {
		return fmt.Errorf("failed to run systemctl daemon-reload: %w", err)
	}
	return nil
}

func (i *UnitInstaller) IsUnitActive(ctx context.Context) (bool, error) {
	if !i.Available() {
		return false, fmt.Errorf("systemd is not available")
	}
	activeOutput, err := i.run(ctx, "is-active", i.config.UnitName)
	if err != nil {
		if IsSystemdNotActiveError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to run systemctl is-active %s: %w", i.config.UnitName, err)
	}
	return ActiveStateBlocksRuntimeStart(ParseUnitActiveState(activeOutput, nil)), nil
}

func (i *UnitInstaller) StopUnit(ctx context.Context) error {
	if !i.Available() {
		return fmt.Errorf("systemd is not available")
	}
	if _, err := i.run(ctx, "stop", i.config.UnitName); err != nil {
		return fmt.Errorf("failed to run systemctl stop %s: %w", i.config.UnitName, err)
	}
	return nil
}

func (i *UnitInstaller) StartUnit(ctx context.Context) error {
	if !i.Available() {
		return fmt.Errorf("systemd is not available")
	}
	if _, err := i.run(ctx, "start", i.config.UnitName); err != nil {
		return fmt.Errorf("failed to run systemctl start %s: %w", i.config.UnitName, err)
	}
	return nil
}

func (i *UnitInstaller) EnableUnit(ctx context.Context) error {
	if !i.Available() {
		return fmt.Errorf("systemd is not available")
	}
	if _, err := i.run(ctx, "enable", i.config.UnitName); err != nil {
		return fmt.Errorf("failed to run systemctl enable %s: %w", i.config.UnitName, err)
	}
	return nil
}

func (i *UnitInstaller) DisableUnit(ctx context.Context) error {
	if !i.Available() {
		return fmt.Errorf("systemd is not available")
	}
	if _, err := i.run(ctx, "disable", i.config.UnitName); err != nil {
		return fmt.Errorf("failed to run systemctl disable %s: %w", i.config.UnitName, err)
	}
	return nil
}

func (i *UnitInstaller) Status(ctx context.Context) (UnitStatus, error) {
	if !i.Available() {
		return UnitStatus{}, fmt.Errorf("systemd is not available")
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

	enabledOutput, err := i.run(ctx, "is-enabled", i.config.UnitName)
	if err != nil {
		if !IsSystemdDisabledError(err) {
			return UnitStatus{}, fmt.Errorf("failed to run systemctl is-enabled %s: %w", i.config.UnitName, err)
		}
	}
	status.UnitFileState = ParseUnitFileState(enabledOutput, err)

	activeOutput, activeErr := i.run(ctx, "is-active", i.config.UnitName)
	if activeErr != nil {
		if !IsSystemdNotActiveError(activeErr) {
			return UnitStatus{}, fmt.Errorf("failed to run systemctl is-active %s: %w", i.config.UnitName, activeErr)
		}
	}
	status.ActiveState = ParseUnitActiveState(activeOutput, activeErr)

	showOutput, showErr := i.run(
		ctx,
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
		if unitBody, readErr := os.ReadFile(i.config.UnitPath); readErr == nil {
			status.Role = DetectUnitRole(string(unitBody))
		}
	}
	return status, nil
}

// run owns the timeout and terminal policy for all systemctl invocations.
func (i *UnitInstaller) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args = append(args, "--no-ask-password", "--no-pager")
	output, err := i.runner.CombinedOutputContext(ctx, "systemctl", args...)
	if ctx.Err() != nil {
		return output, ctx.Err()
	}
	if err != nil && len(output) != 0 {
		return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, err
}
