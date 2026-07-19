package client

type Resolver interface {
	Resolve() (string, error)
}
