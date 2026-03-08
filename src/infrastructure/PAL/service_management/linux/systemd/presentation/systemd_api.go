package presentation

import (
	"tungo/infrastructure/PAL/service_management/linux/systemd/application"
	"tungo/infrastructure/PAL/service_management/linux/systemd/domain"
)

// InstallerFacade is a presentation adapter over application service.
type InstallerFacade struct {
	service *application.Service
}

func NewInstallerFacade(service *application.Service) *InstallerFacade {
	return &InstallerFacade{service: service}
}

func (f *InstallerFacade) Supported() bool {
	return f.service.Supported()
}

func (f *InstallerFacade) InstallServerUnit() (string, error) {
	return f.service.InstallServerUnit()
}

func (f *InstallerFacade) InstallClientUnit() (string, error) {
	return f.service.InstallClientUnit()
}

func (f *InstallerFacade) RemoveUnit() error {
	return f.service.RemoveUnit()
}

func (f *InstallerFacade) IsUnitActive() (bool, error) {
	return f.service.IsUnitActive()
}

func (f *InstallerFacade) StopUnit() error {
	return f.service.StopUnit()
}

func (f *InstallerFacade) StartUnit() error {
	return f.service.StartUnit()
}

func (f *InstallerFacade) EnableUnit() error {
	return f.service.EnableUnit()
}

func (f *InstallerFacade) DisableUnit() error {
	return f.service.DisableUnit()
}

func (f *InstallerFacade) Status() (domain.UnitStatus, error) {
	return f.service.Status()
}
