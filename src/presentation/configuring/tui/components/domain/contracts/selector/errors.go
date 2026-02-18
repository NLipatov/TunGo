package selector

import "errors"

var ErrNavigateBack = errors.New("navigate back requested")
var ErrUserExit = errors.New("user requested tui exit")
