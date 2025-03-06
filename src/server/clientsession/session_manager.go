package clientsession

type SessionManager[K, V any] interface {
	Store(v *V)
	Load(key K) (*V, bool)
	Delete(key K)
}
