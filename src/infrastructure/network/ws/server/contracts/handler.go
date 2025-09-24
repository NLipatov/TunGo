package contracts

import "net/http"

// Handler — HTTP-handlers of incoming connections.
type Handler interface {
	Handle(w http.ResponseWriter, r *http.Request)
}
