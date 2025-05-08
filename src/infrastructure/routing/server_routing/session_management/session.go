package session_management

type ClientSession interface {
	ExternalIP() []byte
	InternalIP() []byte
}
