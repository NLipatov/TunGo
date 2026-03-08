package systemd

import (
	"tungo/infrastructure/PAL/service_management/linux/systemd/domain"
)

func ActiveStateBlocksRuntimeStart(state domain.UnitActiveState) bool {
	return domain.ActiveStateBlocksRuntimeStart(state)
}

func activeStateIndicatesRunning(state domain.UnitActiveState) bool {
	return domain.ActiveStateIndicatesRunning(state)
}

func detectUnitRole(unitBody string) domain.UnitRole {
	return domain.DetectUnitRole(unitBody)
}

func detectUnitRoleFromExecStart(execStart string) domain.UnitRole {
	return domain.DetectUnitRoleFromExecStart(execStart)
}
