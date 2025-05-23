package configuring

import (
	"tungo/domain/mode"
)

type Configurator interface {
	Configure() (mode.Mode, error)
}
