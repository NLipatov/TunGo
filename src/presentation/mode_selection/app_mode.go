package mode_selection

import "tungo/domain/mode"

// AppMode resolves the application's runtime mode.
type AppMode interface {
	Mode() (mode.Mode, error)
}
