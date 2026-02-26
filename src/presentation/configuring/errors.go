package configuring

import "tungo/presentation/ui/tui"

// ErrUserExit is the presentation-level sentinel for user-requested exit
// during configuration, independent from concrete UI wiring in callers.
var ErrUserExit = tui.ErrUserExit

// ErrSessionClosed indicates the unified UI session ended while shutting down.
var ErrSessionClosed = tui.ErrSessionClosed
