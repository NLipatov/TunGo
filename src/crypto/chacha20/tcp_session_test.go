package chacha20

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"testing"
)

// TestDeriveSessionId verifies that a session ID is correctly derived.
func TestDeriveSessionId(t *testing.T) {
	sharedSecret := []byte("this is a shared secret with sufficient length")
	salt := []byte("some salt value")
	sessionID, err := DeriveSessionId(sharedSecret, salt)
	if err != nil {
		t.Fatalf("DeriveSessionId returned error: %v", err)
	}
	// Check that sessionID is not all zeros.
	zero := make([]byte, 32)
	if bytes.Equal(sessionID[:], zero) {
		t.Error("Derived session ID is all zeros")
	}
}

// TestNewTcpSession verifies that a new TcpSession is created without error.
func TestNewTcpSession(t *testing.T) {
	// Create valid 32-byte keys.
	sendKey := make([]byte, 32)
	recvKey := make([]byte, 32)
	if _, err := rand.Read(sendKey); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}
	if _, err := rand.Read(recvKey); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}

	var sessionID [32]byte
	copy(sessionID[:], []byte("static session id for testing!!")) // 32 bytes

	session, err := NewTcpSession(sessionID, sendKey, recvKey, false)
	if err != nil {
		t.Fatalf("NewTcpSession error: %v", err)
	}
	if session.SessionId != sessionID {
		t.Errorf("Expected sessionID %x, got %x", sessionID, session.SessionId)
	}
}

// TestTcpSessionEncryptDecryptRoundTrip simulates a round-trip encryption/decryption between client and server.
func TestTcpSessionEncryptDecryptRoundTrip(t *testing.T) {
	// Prepare static 32-byte keys.
	clientSendKey := bytes.Repeat([]byte{0xAA}, 32)
	clientRecvKey := bytes.Repeat([]byte{0xBB}, 32)
	// On server, keys are swapped.
	serverSendKey := clientRecvKey
	serverRecvKey := clientSendKey

	// Use a static session ID.
	var sessionID [32]byte
	copy(sessionID[:], []byte("static session id for testing!!")) // 32 bytes

	// Create client (isServer=false) and server (isServer=true) sessions.
	clientSession, err := NewTcpSession(sessionID, clientSendKey, clientRecvKey, false)
	if err != nil {
		t.Fatalf("client NewTcpSession error: %v", err)
	}
	serverSession, err := NewTcpSession(sessionID, serverSendKey, serverRecvKey, true)
	if err != nil {
		t.Fatalf("server NewTcpSession error: %v", err)
	}

	plaintext := []byte("Hello, secure world!")
	// Client encrypts the plaintext.
	ciphertext, err := clientSession.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	// Server decrypts the ciphertext.
	decrypted, err := serverSession.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt error: %v", err)
	}
	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypted data does not match original: got %q, want %q", decrypted, plaintext)
	}
}

// TestUseNonceRingBuffer verifies that UseNonceRingBuffer returns the same session.
func TestUseNonceRingBuffer(t *testing.T) {
	var sessionID [32]byte
	copy(sessionID[:], []byte("static session id for testing!!"))
	key := bytes.Repeat([]byte{0xCC}, 32)
	session, err := NewTcpSession(sessionID, key, key, false)
	if err != nil {
		t.Fatalf("NewTcpSession error: %v", err)
	}

	ret := session.UseNonceRingBuffer(100)
	if ret != session {
		t.Error("UseNonceRingBuffer did not return the same session instance")
	}
}

// TestCreateAAD checks that CreateAAD correctly builds additional authentication data.
func TestCreateAAD(t *testing.T) {
	var sessionID [32]byte
	copy(sessionID[:], []byte("static session id for testing!!"))
	key := bytes.Repeat([]byte{0xDD}, 32)
	session, err := NewTcpSession(sessionID, key, key, false)
	if err != nil {
		t.Fatalf("NewTcpSession error: %v", err)
	}

	// Use a sample nonce.
	nonce := []byte("sampleNonce12") // 12 bytes
	// Expected direction for isServer == false is "client-to-server".
	direction := []byte("client-to-server")
	// Build expected AAD: sessionID + direction + nonce.
	expectedAAD := make([]byte, 0, len(session.SessionId)+len(direction)+len(nonce))
	expectedAAD = append(expectedAAD, session.SessionId[:]...)
	expectedAAD = append(expectedAAD, direction...)
	expectedAAD = append(expectedAAD, nonce...)

	// Prepare a buffer with sufficient size.
	aadBuf := make([]byte, 80)
	aad := session.CreateAAD(false, nonce, aadBuf)
	if !bytes.Equal(aad, expectedAAD) {
		t.Errorf("CreateAAD output mismatch.\nExpected: %s\nGot:      %s", hex.EncodeToString(expectedAAD), hex.EncodeToString(aad))
	}

	// Test for server-to-client direction.
	expectedDirection := []byte("server-to-client")
	expectedAAD = make([]byte, 0, len(session.SessionId)+len(expectedDirection)+len(nonce))
	expectedAAD = append(expectedAAD, session.SessionId[:]...)
	expectedAAD = append(expectedAAD, expectedDirection...)
	expectedAAD = append(expectedAAD, nonce...)
	aad = session.CreateAAD(true, nonce, aadBuf)
	if !bytes.Equal(aad, expectedAAD) {
		t.Errorf("CreateAAD (server-to-client) output mismatch.\nExpected: %s\nGot:      %s", hex.EncodeToString(expectedAAD), hex.EncodeToString(aad))
	}
}
