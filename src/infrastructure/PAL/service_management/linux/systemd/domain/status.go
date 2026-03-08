package domain

// UnitStatus is the aggregate projection of service state.
type UnitStatus struct {
	Installed      bool
	Managed        bool
	Role           UnitRole
	LoadState      UnitLoadState
	UnitFileState  UnitFileState
	ActiveState    UnitActiveState
	SubState       string
	Result         string
	ExecMainStatus string
	ExecStart      string
	FragmentPath   string
}
