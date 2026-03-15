package domain

// UnitRole identifies daemon runtime role in unit specification.
type UnitRole string

const (
	UnitRoleUnknown UnitRole = "unknown"
	UnitRoleClient  UnitRole = "client"
	UnitRoleServer  UnitRole = "server"
)

// UnitLoadState mirrors systemd LoadState value.
type UnitLoadState string

const (
	UnitLoadStateUnknown  UnitLoadState = "unknown"
	UnitLoadStateLoaded   UnitLoadState = "loaded"
	UnitLoadStateNotFound UnitLoadState = "not-found"
)

// UnitFileState mirrors systemd unit file state.
type UnitFileState string

const (
	UnitFileStateUnknown  UnitFileState = "unknown"
	UnitFileStateEnabled  UnitFileState = "enabled"
	UnitFileStateDisabled UnitFileState = "disabled"
)

// UnitActiveState mirrors systemd active state.
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
