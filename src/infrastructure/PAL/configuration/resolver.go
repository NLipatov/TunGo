package configuration

// Resolver resolves a configuration file path.
type Resolver interface {
	Resolve() (string, error)
}
