package client_configuration

type Resolver interface {
	Resolve() (string, error)
}
