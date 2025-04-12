package configuration_provider

import (
	"tungo/domain/mode"
)

type Configurator interface {
	Configure() (mode.Mode, error)
}
