package client_configuration

type resolver interface {
	resolve() (string, error)
}
