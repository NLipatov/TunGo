package systemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tungo/internal/commandline"
	"tungo/internal/mode"
	"tungo/internal/platform/command"
)

// Control exposes systemd unit management operations.
type Control interface {
	Setup(context.Context, mode.Mode) (string, error)
	RemoveUnit(context.Context, mode.Mode) error
	IsUnitActive(context.Context, mode.Mode) (bool, error)
	StopUnit(context.Context, mode.Mode) error
	StartUnit(context.Context, mode.Mode) error
	EnableUnit(context.Context, mode.Mode) error
	DisableUnit(context.Context, mode.Mode) error
	Status(context.Context, mode.Mode) (UnitStatus, error)
}

type runner interface {
	CombinedOutputContext(ctx context.Context, name string, args ...string) ([]byte, error)
}

type UnitInstaller struct {
	runner runner
	config Config
}

func NewUnitInstaller(runner runner) *UnitInstaller {
	if runner == nil {
		runner = command.New()
	}
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
	status, err := i.Status(ctx, runtimeMode)
	if err != nil {
		return "", err
	}
	if status.Installed {
		return "", fmt.Errorf("%s is already installed; remove it before installing again", status.Name)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path = i.unitPath(status.Name)
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
			_, disableErr := i.run(cleanupCtx, "disable", status.Name)
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
	if _, err := i.run(ctx, "enable", status.Name); err != nil {
		return "", err
	}
	return path, nil
}

func (i *UnitInstaller) RemoveUnit(ctx context.Context, runtimeMode mode.Mode) error {
	status, err := i.Status(ctx, runtimeMode)
	if err != nil {
		return err
	}

	if !status.Installed {
		return nil
	}
	if !status.Managed {
		return fmt.Errorf("%s is not managed by TunGo", status.Name)
	}
	if _, err := i.run(ctx, "stop", status.Name); err != nil && !IsSystemdNotActiveError(err) {
		return fmt.Errorf("failed to run systemctl stop %s: %w", status.Name, err)
	}
	if _, err := i.run(ctx, "disable", status.Name); err != nil && !IsSystemdDisabledError(err) {
		return fmt.Errorf("failed to run systemctl disable %s: %w", status.Name, err)
	}
	if err := os.Remove(i.unitPath(status.Name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove %s: %w", i.unitPath(status.Name), err)
	}
	if _, err := i.run(ctx, "daemon-reload"); err != nil {
		return fmt.Errorf("failed to run systemctl daemon-reload: %w", err)
	}
	return nil
}

func (i *UnitInstaller) IsUnitActive(ctx context.Context, runtimeMode mode.Mode) (bool, error) {
	status, err := i.Status(ctx, runtimeMode)
	return ActiveStateBlocksRuntimeStart(status.ActiveState), err
}

func (i *UnitInstaller) StopUnit(ctx context.Context, runtimeMode mode.Mode) error {
	status, err := i.Status(ctx, runtimeMode)
	if err != nil {
		return err
	}
	if _, err := i.run(ctx, "stop", status.Name); err != nil {
		return fmt.Errorf("failed to run systemctl stop %s: %w", status.Name, err)
	}
	return nil
}

func (i *UnitInstaller) StartUnit(ctx context.Context, runtimeMode mode.Mode) error {
	status, err := i.Status(ctx, runtimeMode)
	if err != nil {
		return err
	}
	if _, err := i.run(ctx, "start", status.Name); err != nil {
		return fmt.Errorf("failed to run systemctl start %s: %w", status.Name, err)
	}
	return nil
}

func (i *UnitInstaller) EnableUnit(ctx context.Context, runtimeMode mode.Mode) error {
	status, err := i.Status(ctx, runtimeMode)
	if err != nil {
		return err
	}
	if _, err := i.run(ctx, "enable", status.Name); err != nil {
		return fmt.Errorf("failed to run systemctl enable %s: %w", status.Name, err)
	}
	return nil
}

func (i *UnitInstaller) DisableUnit(ctx context.Context, runtimeMode mode.Mode) error {
	status, err := i.Status(ctx, runtimeMode)
	if err != nil {
		return err
	}
	if _, err := i.run(ctx, "disable", status.Name); err != nil {
		return fmt.Errorf("failed to run systemctl disable %s: %w", status.Name, err)
	}
	return nil
}

// Status keeps a legacy tungo.service attached to its existing role. It never
// renames or restarts a service, and rejects ambiguous duplicate installations.
func (i *UnitInstaller) Status(ctx context.Context, runtimeMode mode.Mode) (UnitStatus, error) {
	var role UnitRole
	switch runtimeMode {
	case mode.Client:
		role = UnitRoleClient
	case mode.Server:
		role = UnitRoleServer
	default:
		return UnitStatus{}, fmt.Errorf("invalid daemon mode: %v", runtimeMode)
	}
	status, err := i.unitStatus(ctx, "tungo-"+string(role)+".service")
	if err != nil {
		return UnitStatus{}, err
	}
	legacy, err := i.unitStatus(ctx, "tungo.service")
	if err != nil {
		return UnitStatus{}, err
	}
	if legacy.Installed {
		if legacy.Role == UnitRoleUnknown {
			return UnitStatus{}, fmt.Errorf("cannot determine the role of tungo.service; inspect or remove it with systemctl")
		}
		if legacy.Role == role {
			if status.Installed {
				return UnitStatus{}, fmt.Errorf("both %s and tungo.service exist for %s; remove the duplicate with systemctl", status.Name, role)
			}
			return legacy, nil
		}
	}
	if status.Installed && status.Role != role {
		return UnitStatus{}, fmt.Errorf("%s has an unexpected role; inspect it with systemctl", status.Name)
	}
	return status, nil
}

func (i *UnitInstaller) unitPath(name string) string {
	return filepath.Join(i.config.UnitDir, name)
}

func (i *UnitInstaller) unitStatus(ctx context.Context, unitName string) (UnitStatus, error) {
	if !i.Available() {
		return UnitStatus{}, fmt.Errorf("systemd is not available")
	}

	status := UnitStatus{
		Name:           unitName,
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

	enabledOutput, err := i.run(ctx, "is-enabled", unitName)
	if err != nil {
		if !IsSystemdDisabledError(err) {
			return UnitStatus{}, fmt.Errorf("failed to run systemctl is-enabled %s: %w", unitName, err)
		}
	}
	status.UnitFileState = ParseUnitFileState(enabledOutput, err)

	activeOutput, activeErr := i.run(ctx, "is-active", unitName)
	if activeErr != nil {
		if !IsSystemdNotActiveError(activeErr) {
			return UnitStatus{}, fmt.Errorf("failed to run systemctl is-active %s: %w", unitName, activeErr)
		}
	}
	status.ActiveState = ParseUnitActiveState(activeOutput, activeErr)

	showOutput, showErr := i.run(
		ctx,
		"show",
		unitName,
		"--property=LoadState,ActiveState,SubState,Result,ExecMainStatus,ExecStart,FragmentPath",
		"--no-page",
	)
	if showErr != nil {
		return UnitStatus{}, fmt.Errorf("failed to run systemctl show %s: %w", unitName, showErr)
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
	status.Managed = IsInstallerManagedFragmentPath(status.FragmentPath, i.unitPath(unitName))

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
		if unitBody, readErr := os.ReadFile(i.unitPath(unitName)); readErr == nil {
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
