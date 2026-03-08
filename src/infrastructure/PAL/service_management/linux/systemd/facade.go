package systemd

import (
	"tungo/infrastructure/PAL/exec_commander"
	app "tungo/infrastructure/PAL/service_management/linux/systemd/application"
	"tungo/infrastructure/PAL/service_management/linux/systemd/domain"
	infra "tungo/infrastructure/PAL/service_management/linux/systemd/infrastructure"
	"tungo/infrastructure/PAL/service_management/linux/systemd/presentation"
)

// UnitInstaller is a compatibility facade for callers using package systemd.
type UnitInstaller struct {
	commander exec_commander.Commander
	config    infra.Config
}

func NewUnitInstaller(commander exec_commander.Commander) *UnitInstaller {
	if commander == nil {
		commander = exec_commander.NewExecCommander()
	}
	return &UnitInstaller{
		commander: commander,
		config:    defaultSystemdConfig,
	}
}

func (i *UnitInstaller) Supported() bool {
	return i.presenter().Supported()
}

func (i *UnitInstaller) InstallServerUnit() (string, error) {
	return i.presenter().InstallServerUnit()
}

func (i *UnitInstaller) InstallClientUnit() (string, error) {
	return i.presenter().InstallClientUnit()
}

func (i *UnitInstaller) RemoveUnit() error {
	return i.presenter().RemoveUnit()
}

func (i *UnitInstaller) IsUnitActive() (bool, error) {
	return i.presenter().IsUnitActive()
}

func (i *UnitInstaller) StopUnit() error {
	return i.presenter().StopUnit()
}

func (i *UnitInstaller) StartUnit() error {
	return i.presenter().StartUnit()
}

func (i *UnitInstaller) EnableUnit() error {
	return i.presenter().EnableUnit()
}

func (i *UnitInstaller) DisableUnit() error {
	return i.presenter().DisableUnit()
}

func (i *UnitInstaller) Status() (domain.UnitStatus, error) {
	return i.presenter().Status()
}

func (i *UnitInstaller) presenter() *presentation.Systemd {
	service := app.NewService(i.commander, i.hooks(), i.config)
	return presentation.NewSystemd(service)
}

func (i *UnitInstaller) hooks() infra.Hooks {
	return infra.Hooks{
		Stat:      statPath,
		Lstat:     lstatPath,
		LookPath:  lookPath,
		WriteFile: writeFilePath,
		ReadFile:  readFilePath,
		Remove:    removePath,
		Geteuid:   geteuid,
	}
}
