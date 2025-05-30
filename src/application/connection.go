package application

type Connection interface {
	Establish() (ConnectionAdapter, error)
}
