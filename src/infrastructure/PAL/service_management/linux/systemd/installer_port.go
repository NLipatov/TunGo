package systemd

import "tungo/infrastructure/PAL/service_management/linux/systemd/domain"

// Installer is the public package port used by outer layers.
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
	Status() (domain.UnitStatus, error)
}
