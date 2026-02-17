package session

import "sync"

// SessionRevoker provides capability to revoke sessions by public key.
// Used by ConfigWatcher to terminate sessions when AllowedPeers changes.
type SessionRevoker interface {
	// RevokeByPubKey terminates all sessions for the given public key.
	// Returns the total number of sessions terminated.
	RevokeByPubKey(pubKey []byte) int
}

// CompositeSessionRevoker aggregates multiple repositories and revokes
// sessions across all of them. Thread-safe for concurrent registration.
type CompositeSessionRevoker struct {
	mu    sync.RWMutex
	repos []RepositoryWithRevocation
}

// NewCompositeSessionRevoker creates a new composite revoker.
func NewCompositeSessionRevoker() *CompositeSessionRevoker {
	return &CompositeSessionRevoker{
		repos: make([]RepositoryWithRevocation, 0),
	}
}

// Register adds a repository to the composite revoker.
// Thread-safe, can be called while RevokeByPubKey is running.
func (c *CompositeSessionRevoker) Register(repo RepositoryWithRevocation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.repos = append(c.repos, repo)
}

// RevokeByPubKey terminates sessions for the given public key across all
// registered repositories. Returns total count of terminated sessions.
func (c *CompositeSessionRevoker) RevokeByPubKey(pubKey []byte) int {
	c.mu.RLock()
	repos := make([]RepositoryWithRevocation, len(c.repos))
	copy(repos, c.repos)
	c.mu.RUnlock()

	total := 0
	for _, repo := range repos {
		total += repo.TerminateByPubKey(pubKey)
	}
	return total
}
