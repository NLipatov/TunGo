package client

import (
	"path/filepath"

	"tungo/internal/product"
)

func defaultPath() string {
	return filepath.Join(product.ConfigurationDirectory(), "client_configuration.json")
}
