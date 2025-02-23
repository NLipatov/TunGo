package chacha20

import (
	"bytes"
	"crypto/rand"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"testing"
)

func TestDeriveSessionId(t *testing.T) {
	sharedSecret := make([]byte, 32)
	salt := make([]byte, 16)
	_, _ = rand.Read(sharedSecret)
	_, _ = rand.Read(salt)

	_, err := DeriveSessionId(sharedSecret, salt)
	if err != nil {
		t.Fatalf("unexpected error during session ID derivation: %v", err)
	}
}

func TestNewSession(t *testing.T) {
	sendKey := make([]byte, 32)
	recvKey := make([]byte, 32)
	_, _ = rand.Read(sendKey)
	_, _ = rand.Read(recvKey)

	session, err := NewSession(sendKey, recvKey, true)
	if err != nil {
		t.Fatalf("unexpected error during session creation: %v", err)
	}

	if session.sendCipher == nil || session.recvCipher == nil {
		t.Errorf("sendCipher or recvCipher not initialized")
	}
	if session.SendNonce == nil || session.RecvNonce == nil {
		t.Errorf("SendNonce or RecvNonce not initialized")
	}
}

func TestSession_ClientServerEncryption(t *testing.T) {
	serverSession, clientSession := createServerAndClienSessions(t)

	plaintext := []byte("Hello, secure world!")

	ciphertext, clientNonce, err := clientSession.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Client encryption failed: %v", err)
	}

	if bytes.Contains(ciphertext, plaintext) {
		t.Fatalf("ciphertext must not contain plaintext as a subarray")
	}

	decrypted, serverNonce, err := serverSession.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Server decryption failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypted text mismatch: expected %s, got %s", plaintext, decrypted)
	}

	if !bytes.Equal(clientNonce.Encode(), serverNonce.Encode()) {
		t.Errorf("Nonce mismatch: expected %v, got %v", clientNonce.Encode(), serverNonce.Encode())
	}

	serverCiphertext, serverNonce, err := serverSession.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Server encryption failed: %v", err)
	}

	if bytes.Contains(serverCiphertext, plaintext) {
		t.Fatalf("ciphertext must not contain plaintext as a subarray")
	}

	clientDecrypted, clientNonce, err := clientSession.Decrypt(serverCiphertext)
	if err != nil {
		t.Fatalf("Client decryption failed: %v", err)
	}

	if !bytes.Equal(plaintext, clientDecrypted) {
		t.Errorf("Decrypted text mismatch: expected %s, got %s", plaintext, clientDecrypted)
	}

	if !bytes.Equal(serverNonce.Encode(), clientNonce.Encode()) {
		t.Errorf("Nonce mismatch: expected %v, got %v", serverNonce.Encode(), clientNonce.Encode())
	}
}

func TestSession_ClientServerEncryption_ServerDecryptsViaNonceBuf(t *testing.T) {
	serverSession, clientSession := createServerAndClienSessions(t)

	serverSession.UseNonceRingBuffer(2096)
	clientSession.UseNonceRingBuffer(2096)

	plaintext := []byte("Hello, secure world!")

	ciphertext, clientNonce, err := clientSession.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Client encryption failed: %v", err)
	}

	if bytes.Contains(ciphertext, plaintext) {
		t.Fatalf("ciphertext must not contain plaintext as a subarray")
	}

	decrypted, err := serverSession.DecryptViaNonceBuf(ciphertext, clientNonce)
	if err != nil {
		t.Fatalf("Server decryption failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypted text mismatch: expected %s, got %s", plaintext, decrypted)
	}
}

func TestSession_ClientServerEncryption_ClientDecryptsViaNonceBuf(t *testing.T) {
	serverSession, clientSession := createServerAndClienSessions(t)

	serverSession.UseNonceRingBuffer(2096)
	clientSession.UseNonceRingBuffer(2096)

	plaintext := []byte("Hello, secure world!")

	ciphertext, serverNonce, err := serverSession.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Server encryption failed: %v", err)
	}

	if bytes.Contains(ciphertext, plaintext) {
		t.Fatalf("ciphertext must not contain plaintext as a subarray")
	}

	decrypted, err := clientSession.DecryptViaNonceBuf(ciphertext, serverNonce)
	if err != nil {
		t.Fatalf("Client decryption failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypted text mismatch: expected %s, got %s", plaintext, decrypted)
	}
}

func createServerAndClienSessions(t *testing.T) (*SessionImpl, *SessionImpl) {
	clientPrivate := make([]byte, 32)
	serverPrivate := make([]byte, 32)
	_, _ = rand.Read(clientPrivate)
	_, _ = rand.Read(serverPrivate)

	clientPublic, _ := curve25519.X25519(clientPrivate, curve25519.Basepoint)
	serverPublic, _ := curve25519.X25519(serverPrivate, curve25519.Basepoint)

	clientSharedSecret, _ := curve25519.X25519(clientPrivate, serverPublic)
	serverSharedSecret, _ := curve25519.X25519(serverPrivate, clientPublic)

	if !bytes.Equal(clientSharedSecret, serverSharedSecret) {
		t.Fatalf("Shared secrets do not match")
	}

	keySize := chacha20poly1305.KeySize
	clientToServerKey := make([]byte, keySize)
	serverToClientKey := make([]byte, keySize)

	_, _ = rand.Read(clientToServerKey)
	_, _ = rand.Read(serverToClientKey)

	clientSession, err := NewSession(clientToServerKey, serverToClientKey, false)
	if err != nil {
		t.Fatalf("Failed to create client session: %v", err)
	}

	serverSession, err := NewSession(serverToClientKey, clientToServerKey, true)
	if err != nil {
		t.Fatalf("Failed to create server session: %v", err)
	}

	return serverSession, clientSession
}

func TestSession_CreateAAD(t *testing.T) {
	sessionID := [32]byte{}
	copy(sessionID[:], "test-session-id-32-bytes-long")

	session := &SessionImpl{
		SessionId: sessionID,
	}

	nonce := []byte{0, 1, 2, 3, 4, 5, 6, 7}
	aad := session.CreateAAD(true, nonce)

	expectedPrefix := append(sessionID[:], []byte("server-to-client")...)
	expectedAAD := append(expectedPrefix, nonce...)

	if !bytes.Equal(aad, expectedAAD) {
		t.Errorf("AAD mismatch: expected %v, got %v", expectedAAD, aad)
	}
}

func TestSession_UseNonceRingBufferSize(t *testing.T) {
	sendKey := make([]byte, 32)
	recvKey := make([]byte, 32)
	_, _ = rand.Read(sendKey)
	_, _ = rand.Read(recvKey)

	session, _ := NewSession(sendKey, recvKey, true)
	session.UseNonceRingBuffer(2096)

	if session.nonceBuf == nil {
		t.Fatalf("nonceBuf not initialized")
	}
	if session.nonceBuf.size != 2096 {
		t.Errorf("nonceBuf size mismatch: expected 2096, got %d", session.nonceBuf.size)
	}
}

func TestSession_UseNonceRingBufferSize_SmallSize(t *testing.T) {
	sendKey := make([]byte, 32)
	recvKey := make([]byte, 32)
	_, _ = rand.Read(sendKey)
	_, _ = rand.Read(recvKey)

	session, _ := NewSession(sendKey, recvKey, true)
	session.UseNonceRingBuffer(512)

	if session.nonceBuf == nil {
		t.Fatalf("nonceBuf not initialized")
	}
	if session.nonceBuf.size != 1024 {
		t.Errorf("nonceBuf size mismatch: expected 1024, got %d", session.nonceBuf.size)
	}
}
