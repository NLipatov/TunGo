package client

import "tungo/infrastructure/PAL/configuration"

// Resolver is an alias for configuration.Resolver so that existing
// code inside the client package compiles without changes.
type Resolver = configuration.Resolver
