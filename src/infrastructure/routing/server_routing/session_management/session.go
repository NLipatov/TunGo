package session_management

type ClientSession interface {
	ExternalIP() [4]byte
	InternalIP() [4]byte
}
