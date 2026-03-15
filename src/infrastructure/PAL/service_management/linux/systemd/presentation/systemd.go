package presentation

import (
	"tungo/infrastructure/PAL/service_management/linux/systemd/application"
	"tungo/infrastructure/PAL/service_management/linux/systemd/domain"
)

// Systemd is a presentation adapter over application service.
type Systemd struct {
	service *application.Service
}

func NewSystemd(service *application.Service) *Systemd {
	return &Systemd{service: service}
}

func (f *Systemd) Supported() bool {
	return f.service.Supported()
}

func (f *Systemd) InstallServerUnit() (string, error) {
	return f.service.InstallServerUnit()
}

func (f *Systemd) InstallClientUnit() (string, error) {
	return f.service.InstallClientUnit()
}

func (f *Systemd) RemoveUnit() error {
	return f.service.RemoveUnit()
}

func (f *Systemd) IsUnitActive() (bool, error) {
	return f.service.IsUnitActive()
}

func (f *Systemd) StopUnit() error {
	return f.service.StopUnit()
}

func (f *Systemd) StartUnit() error {
	return f.service.StartUnit()
}

func (f *Systemd) EnableUnit() error {
	return f.service.EnableUnit()
}

func (f *Systemd) DisableUnit() error {
	return f.service.DisableUnit()
}

func (f *Systemd) Status() (domain.UnitStatus, error) {
	return f.service.Status()
}
