package noise

import (
	"crypto/hmac"
	"crypto/rand"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/crypto/blake2s"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// CookieSize is the size of the cookie value.
	CookieSize = 16

	// CookieNonceSize is the nonce size for XChaCha20-Poly1305.
	CookieNonceSize = 24

	// CookieReplySize is the total size of an encrypted cookie reply.
	// nonce (24) + ciphertext (16) + tag (16) = 56 bytes
	CookieReplySize = CookieNonceSize + CookieSize + chacha20poly1305.Overhead

	// CookieBucketSeconds is the time window for cookie validity.
	CookieBucketSeconds = 120
)

// CookieManager handles cookie generation, encryption, and validation
// for DoS protection during handshake.
type CookieManager struct {
	mu     sync.RWMutex
	secret [32]byte
	now    func() time.Time
}

// NewCookieManager creates a new CookieManager with a random secret.
func NewCookieManager() (*CookieManager, error) {
	cm := &CookieManager{
		now: time.Now,
	}
	if _, err := rand.Read(cm.secret[:]); err != nil {
		return nil, err
	}
	return cm, nil
}

// NewCookieManagerWithSecret creates a CookieManager with a specific secret (for testing).
func NewCookieManagerWithSecret(secret [32]byte) *CookieManager {
	return &CookieManager{
		secret: secret,
		now:    time.Now,
	}
}

// ComputeCookieValue computes the cookie value for a client IP.
// cookie_value = BLAKE2s-128(key=server_cookie_secret, client_ip || timestamp_bucket)
// Uses BLAKE2s's built-in keyed MAC mode (more efficient than HMAC).
func (cm *CookieManager) ComputeCookieValue(clientIP netip.Addr) []byte {
	cm.mu.RLock()
	secret := cm.secret
	cm.mu.RUnlock()

	bucket := cm.now().Unix() / CookieBucketSeconds
	ip16 := clientIP.As16()
	data := make([]byte, 0, 18) // 16 bytes IP + 2 bytes timestamp
	data = append(data, ip16[:]...)
	data = append(data, byte(bucket), byte(bucket>>8))

	// BLAKE2s with key - returns keyed 128-bit MAC
	h, _ := blake2s.New128(secret[:])
	h.Write(data)
	return h.Sum(nil) // 16 bytes
}

// deriveCookieEncryptionKey derives the key for cookie encryption.
// key = BLAKE2s(cookie_label || protocol_id || version || server_pubkey || client_ephemeral)
func deriveCookieEncryptionKey(serverPubKey, clientEphemeral []byte) [32]byte {
	h, _ := blake2s.New256(nil)
	h.Write([]byte(CookieLabel))
	h.Write([]byte(ProtocolID))
	h.Write([]byte{byte(ProtocolVersion)})
	h.Write(serverPubKey)
	h.Write(clientEphemeral)
	var key [32]byte
	copy(key[:], h.Sum(nil))
	return key
}

// CreateCookieReply creates an encrypted cookie reply for the client.
// The cookie is encrypted with XChaCha20-Poly1305 using a key derived from
// the server's public key and client's ephemeral key.
// Format: nonce (24) || encrypted_cookie (16 + 16 tag)
func (cm *CookieManager) CreateCookieReply(clientIP netip.Addr, clientEphemeral, serverPubKey []byte) ([]byte, error) {
	cookieValue := cm.ComputeCookieValue(clientIP)

	key := deriveCookieEncryptionKey(serverPubKey, clientEphemeral)
	defer ZeroBytes(key[:])

	aead, err := chacha20poly1305.NewX(key[:])
	if err != nil {
		return nil, err
	}

	var nonce [CookieNonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}

	reply := make([]byte, CookieNonceSize+aead.Overhead()+len(cookieValue))
	copy(reply[:CookieNonceSize], nonce[:])
	aead.Seal(reply[CookieNonceSize:CookieNonceSize], nonce[:], cookieValue, nil)

	return reply, nil
}

// DecryptCookieReply decrypts a cookie reply received from the server.
// Used by the client after receiving a cookie challenge.
func DecryptCookieReply(reply, clientEphemeral, serverPubKey []byte) ([]byte, error) {
	if len(reply) < CookieNonceSize+chacha20poly1305.Overhead+1 {
		return nil, ErrInvalidCookieReply
	}

	nonce := reply[:CookieNonceSize]
	ciphertext := reply[CookieNonceSize:]

	key := deriveCookieEncryptionKey(serverPubKey, clientEphemeral)
	defer ZeroBytes(key[:])

	aead, err := chacha20poly1305.NewX(key[:])
	if err != nil {
		return nil, err
	}

	return aead.Open(nil, nonce, ciphertext, nil)
}

// ValidateCookie checks if a cookie is valid for the given client IP.
// Checks both current and previous time bucket to handle clock drift.
func (cm *CookieManager) ValidateCookie(clientIP netip.Addr, cookie []byte) bool {
	// Check current bucket
	expected := cm.ComputeCookieValue(clientIP)
	if hmac.Equal(cookie, expected) {
		return true
	}

	// Check previous bucket (transition period)
	cm.mu.RLock()
	secret := cm.secret
	cm.mu.RUnlock()

	bucket := cm.now().Unix()/CookieBucketSeconds - 1
	ip16 := clientIP.As16()
	data := make([]byte, 0, 18)
	data = append(data, ip16[:]...)
	data = append(data, byte(bucket), byte(bucket>>8))

	h, _ := blake2s.New128(secret[:])
	h.Write(data)
	expectedPrev := h.Sum(nil)

	return hmac.Equal(cookie, expectedPrev)
}

// RotateSecret generates a new random secret.
// Old cookies will become invalid after the bucket transition period.
func (cm *CookieManager) RotateSecret() error {
	var newSecret [32]byte
	if _, err := rand.Read(newSecret[:]); err != nil {
		return err
	}

	cm.mu.Lock()
	oldSecret := cm.secret
	cm.secret = newSecret
	cm.mu.Unlock()

	ZeroBytes(oldSecret[:])
	return nil
}

// IsCookieReply checks if a response is a cookie reply based on its size.
// Cookie replies are exactly CookieReplySize bytes.
func IsCookieReply(response []byte) bool {
	return len(response) == CookieReplySize
}
