package service

type Handler interface {
	Handle(data []byte) []byte
}
