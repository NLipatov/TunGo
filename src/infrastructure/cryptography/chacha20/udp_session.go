package chacha20

import (
	"crypto/cipher"
	"fmt"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

type (
	DefaultUdpSession struct {
		SessionId [32]byte
		encoder   DefaultUDPEncoder
		isServer  bool

		current    keySlot
		next       keySlot
		previous   keySlot
		prevExpiry time.Time

		pendingRekeyPriv *[32]byte
		encryptionAadBuf [aadLength]byte
		decryptionAadBuf [aadLength]byte
	}
)

type keySlot struct {
	send    cipher.AEAD
	recv    cipher.AEAD
	sendKey []byte
	recvKey []byte
	keyID   uint8
	nonce   *Nonce
	window  *Sliding64
	set     bool
}

func NewUdpSession(id [32]byte, sendKey, recvKey []byte, isServer bool) (*DefaultUdpSession, error) {
	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return nil, err
	}

	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return nil, err
	}

	curNonce := NewNonce()
	return &DefaultUdpSession{
		SessionId:  id,
		isServer:   isServer,
		encoder:    DefaultUDPEncoder{},
		prevExpiry: time.Time{},
		current: keySlot{
			send:    sendCipher,
			recv:    recvCipher,
			sendKey: append([]byte(nil), sendKey...),
			recvKey: append([]byte(nil), recvKey...),
			keyID:   0,
			nonce:   curNonce,
			window:  NewSliding64(),
			set:     true,
		},
	}, nil
}

func (s *DefaultUdpSession) Encrypt(plaintext []byte) ([]byte, error) {
	// guarantee inplace encryption
	if cap(plaintext) < len(plaintext)+chacha20poly1305.Overhead {
		return nil, fmt.Errorf("insufficient capacity for in-place encryption: len=%d, cap=%d",
			len(plaintext), cap(plaintext))
	}

	// buf: [1B keyID | 12B nonce space | payload ...]
	if len(plaintext) < chacha20poly1305.NonceSize+1 {
		return nil, fmt.Errorf("encrypt: buffer too short: %d", len(plaintext))
	}

	slot := &s.current
	if err := slot.nonce.incrementNonce(); err != nil {
		return nil, err
	}

	// 1) write keyID + nonce into the first 13 bytes
	plaintext[0] = slot.keyID
	nonce := plaintext[1 : 1+chacha20poly1305.NonceSize]
	_ = slot.nonce.Encode(nonce)

	// 2) build AAD = sessionId || direction || nonce
	aad := s.CreateAAD(s.isServer, plaintext[:1+chacha20poly1305.NonceSize], s.encryptionAadBuf[:])

	// 3) plaintext is everything after the 12B header
	plain := plaintext[1+chacha20poly1305.NonceSize:]

	// 4) in-place encrypt: ciphertext overwrites plaintext region
	//    requires caller to allocate +Overhead capacity
	ct := slot.send.Seal(plain[:0], nonce, plain, aad)

	// 5) return header + ciphertext view
	return plaintext[:1+chacha20poly1305.NonceSize+len(ct)], nil
}

func (s *DefaultUdpSession) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < 1+chacha20poly1305.NonceSize+chacha20poly1305.Overhead {
		return nil, fmt.Errorf("cipher too short: %d", len(ciphertext))
	}
	keyID := ciphertext[0]
	nonceBytes := ciphertext[1 : 1+chacha20poly1305.NonceSize]
	payloadBytes := ciphertext[1+chacha20poly1305.NonceSize:]

	// Try keys by id: current, next, previous
	if pt, ok := s.tryDecrypt(&s.current, keyID, nonceBytes, payloadBytes); ok {
		return pt, nil
	}
	if s.next.set {
		if pt, ok := s.tryDecrypt(&s.next, keyID, nonceBytes, payloadBytes); ok {
			s.promoteNext()
			return pt, nil
		}
	}
	if s.previous.set {
		// expire previous after grace window
		if !s.prevExpiry.IsZero() && time.Now().After(s.prevExpiry) {
			s.previous = keySlot{}
		} else if pt, ok := s.tryDecrypt(&s.previous, keyID, nonceBytes, payloadBytes); ok {
			return pt, nil
		}
	}
	return nil, fmt.Errorf("failed to decrypt: %w", ErrNonUniqueNonce)
}

func (s *DefaultUdpSession) CreateAAD(isServerToClient bool, header13, aad []byte) []byte {
	// header13 = keyID(1) + nonce(12)
	copy(aad[:sessionIdentifierLength], s.SessionId[:]) // 0..32
	if isServerToClient {
		copy(aad[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirS2C[:]) // 32..48
	} else {
		copy(aad[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirC2S[:]) // 32..48
	}
	aad[sessionIdentifierLength+directionLength] = header13[0]                   // keyID
	copy(aad[sessionIdentifierLength+directionLength+1:aadLength], header13[1:]) // nonce
	return aad[:aadLength]
}

// ClientToServerKey returns the key used for C->S traffic.
// For server instances this is recvKey; for clients it is sendKey.
func (s *DefaultUdpSession) ClientToServerKey() []byte {
	if s.isServer {
		return s.current.recvKey
	}
	return s.current.sendKey
}

// ServerToClientKey returns the key used for S->C traffic.
// For server instances this is sendKey; for clients it is recvKey.
func (s *DefaultUdpSession) ServerToClientKey() []byte {
	if s.isServer {
		return s.current.sendKey
	}
	return s.current.recvKey
}

func (s *DefaultUdpSession) CurrentKeyID() uint8 {
	return s.current.keyID
}
func (s *DefaultUdpSession) CurrentEpoch() uint8 { // alias for compatibility
	return s.current.keyID
}

func (s *DefaultUdpSession) SetPendingRekeyPrivateKey(priv [32]byte) {
	s.pendingRekeyPriv = &priv
}

func (s *DefaultUdpSession) PendingRekeyPrivateKey() ([32]byte, bool) {
	if s.pendingRekeyPriv == nil {
		return [32]byte{}, false
	}
	return *s.pendingRekeyPriv, true
}

func (s *DefaultUdpSession) ClearPendingRekeyPrivateKey() {
	s.pendingRekeyPriv = nil
}

// InstallNextKeys sets the next epoch keys and ciphers.
func (s *DefaultUdpSession) InstallNextKeys(keyID uint8, sendKey, recvKey []byte) error {
	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return err
	}
	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return err
	}
	n := NewNonce()
	s.next = keySlot{
		send:    sendCipher,
		recv:    recvCipher,
		sendKey: append([]byte(nil), sendKey...),
		recvKey: append([]byte(nil), recvKey...),
		keyID:   keyID,
		nonce:   n,
		window:  NewSliding64(),
		set:     true,
	}
	return nil
}

func (s *DefaultUdpSession) tryDecrypt(slot *keySlot, keyID byte, nonceBytes, payloadBytes []byte) ([]byte, bool) {
	if slot.recv == nil || !slot.set {
		return nil, false
	}
	if keyID != slot.keyID {
		return nil, false
	}
	var n12 [chacha20poly1305.NonceSize]byte
	copy(n12[:], nonceBytes)
	if err := slot.window.Validate(n12); err != nil {
		return nil, false
	}
	header := append([]byte{keyID}, nonceBytes...)
	aad := s.CreateAAD(!s.isServer, header, s.decryptionAadBuf[:])
	pt, err := slot.recv.Open(payloadBytes[:0], nonceBytes, payloadBytes, aad)
	if err != nil {
		return nil, false
	}
	return pt, true
}

func (s *DefaultUdpSession) promoteNext() {
	s.previous = s.current
	s.previous.set = true
	s.prevExpiry = time.Now().Add(2 * time.Minute)
	s.current = s.next
	s.current.set = true
	s.next = keySlot{}
}
