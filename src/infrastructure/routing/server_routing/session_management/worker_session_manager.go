package session_management

type WorkerSessionManager[SessionType any] interface {
	Add(session SessionType)
	Delete(session SessionType)
	GetByInternalIP(ip []byte) (SessionType, bool)
	GetByExternalIP(ip []byte) (SessionType, bool)
}
