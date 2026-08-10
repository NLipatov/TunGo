//go:build darwin

package route

import (
	"tungo/internal/platform/command"
)

func NewV4(runner command.Runner) Contract {
	return newV4(runner)
}

func NewV6(runner command.Runner) Contract {
	return newV6(runner)
}
