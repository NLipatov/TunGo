package chacha20

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

func randKey() []byte {
	k := make([]byte, chacha20poly1305.KeySize)
	_, _ = rand.Read(k)
	return k
}

func randID() [32]byte {
	var id [32]byte
	_, _ = rand.Read(id[:])
	return id
}

func TestNewTcpCryptographyService_KeyLen(t *testing.T) {
	id := randID()
	short := make([]byte, 5)
	long := randKey()

	if _, err := NewTcpCryptographyService(id, short, long, false); err == nil {
		t.Fatal("expected error for short sendKey")
	}
	if _, err := NewTcpCryptographyService(id, long, short, false); err == nil {
		t.Fatal("expected error for short recvKey")
	}
}

func TestTcpSession_Encrypt_InPlaceCapacityError(t *testing.T) {
	id := randID()
	key := randKey()

	s, err := NewTcpCryptographyService(id, key, key, false)
	if err != nil {
		t.Fatalf("NewTcpCryptographyService: %v", err)
	}

	// Not enough cap for in-place encryption (need +Overhead)
	msg := make([]byte, 32) // len=32, cap=32
	if _, err := s.Encrypt(msg); err == nil {
		t.Fatal("expected insufficient capacity error")
	}
}

func TestTcpSession_RoundTrip_And_Replay(t *testing.T) {
	id := randID()
	key := randKey()

	// client -> server
	client, err := NewTcpCryptographyService(id, key, key, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	server, err := NewTcpCryptographyService(id, key, key, true)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	// Grow backing array to allow in-place encryption
	msg := []byte("secret payload")
	buf := make([]byte, len(msg), len(msg)+chacha20poly1305.Overhead)
	copy(buf, msg)

	ct1, err := client.Encrypt(buf)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	pt, err := server.Decrypt(ct1)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(pt, msg) {
		t.Fatalf("round-trip mismatch: got %q want %q", pt, msg)
	}

	// Replay should fail because deterministic counter has advanced.
	if _, err := server.Decrypt(ct1); err == nil {
		t.Fatalf("expected replay to fail with moved counter")
	}
}

func TestTcpSession_Encrypt_ChangesWithNonce(t *testing.T) {
	id := randID()
	key := randKey()

	cli, err := NewTcpCryptographyService(id, key, key, false)
	if err != nil {
		t.Fatalf("NewTcpCryptographyService: %v", err)
	}

	// prepare same message content with enough cap
	msg1 := make([]byte, 16, 16+chacha20poly1305.Overhead)
	msg2 := make([]byte, 16, 16+chacha20poly1305.Overhead)
	copy(msg1, "same-plaintext---")
	copy(msg2, "same-plaintext---")

	ct1, err := cli.Encrypt(msg1)
	if err != nil {
		t.Fatalf("Encrypt #1: %v", err)
	}
	ct2, err := cli.Encrypt(msg2)
	if err != nil {
		t.Fatalf("Encrypt #2: %v", err)
	}

	if bytes.Equal(ct1, ct2) {
		t.Fatal("ciphertexts should differ when nonce increments")
	}
}

func TestTcpSession_DifferentSessionID_Fails(t *testing.T) {
	// Different SessionId -> AAD mismatch -> decryption must fail
	idClient := randID()
	idServer := randID()
	key := randKey()

	client, err := NewTcpCryptographyService(idClient, key, key, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	server, err := NewTcpCryptographyService(idServer, key, key, true)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	msg := make([]byte, 8, 8+chacha20poly1305.Overhead)
	copy(msg, "payload!")
	ct, err := client.Encrypt(msg)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := server.Decrypt(ct); err == nil {
		t.Fatal("expected decryption error with different SessionId, got nil")
	}
}

func TestCreateAAD_BothDirections(t *testing.T) {
	// Use constructor to ensure AAD buffers are pre-filled correctly.
	id := randID()
	key := randKey()

	// Test client session (encrypts C2S, decrypts S2C)
	clientSession, err := NewTcpCryptographyService(id, key, key, false)
	if err != nil {
		t.Fatalf("NewTcpCryptographyService: %v", err)
	}

	// Test server session (encrypts S2C, decrypts C2S)
	serverSession, err := NewTcpCryptographyService(id, key, key, true)
	if err != nil {
		t.Fatalf("NewTcpCryptographyService: %v", err)
	}

	nonce := make([]byte, chacha20poly1305.NonceSize)
	for i := range nonce {
		nonce[i] = byte(i + 1)
	}

	// Client encrypts C2S - uses encryptionAadBuf
	aadC2S := clientSession.CreateAAD(false, nonce, clientSession.encryptionAadBuf[:])
	if len(aadC2S) != aadLength {
		t.Fatalf("aad len=%d, want %d", len(aadC2S), aadLength)
	}
	if !bytes.Equal(aadC2S[:sessionIdentifierLength], id[:]) {
		t.Fatal("session id part mismatch (C2S)")
	}
	if !bytes.Equal(aadC2S[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirC2S[:]) {
		t.Fatal("direction bytes mismatch (C2S)")
	}
	if !bytes.Equal(aadC2S[sessionIdentifierLength+directionLength:aadLength], nonce) {
		t.Fatal("nonce bytes mismatch (C2S)")
	}

	// Server encrypts S2C - uses encryptionAadBuf
	aadS2C := serverSession.CreateAAD(true, nonce, serverSession.encryptionAadBuf[:])
	if len(aadS2C) != aadLength {
		t.Fatalf("aad len=%d, want %d", len(aadS2C), aadLength)
	}
	if !bytes.Equal(aadS2C[:sessionIdentifierLength], id[:]) {
		t.Fatal("session id part mismatch (S2C)")
	}
	if !bytes.Equal(aadS2C[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirS2C[:]) {
		t.Fatal("direction bytes mismatch (S2C)")
	}
	if !bytes.Equal(aadS2C[sessionIdentifierLength+directionLength:aadLength], nonce) {
		t.Fatal("nonce bytes mismatch (S2C)")
	}

	// Direction slices must differ
	if bytes.Equal(aadC2S[sessionIdentifierLength:sessionIdentifierLength+directionLength],
		aadS2C[sessionIdentifierLength:sessionIdentifierLength+directionLength]) {
		t.Fatal("C2S and S2C direction segments must differ")
	}
}

// ---- TcpCrypto (epoch-based) tests ----

func newCryptoPair(t *testing.T) (client, server *TcpCrypto) {
	t.Helper()
	id := randID()
	keyC2S := randKey()
	keyS2C := randKey()

	c2sCipher, err := chacha20poly1305.New(keyC2S)
	if err != nil {
		t.Fatalf("new c2s cipher: %v", err)
	}
	s2cCipher, err := chacha20poly1305.New(keyS2C)
	if err != nil {
		t.Fatalf("new s2c cipher: %v", err)
	}

	client = NewTcpCrypto(id, c2sCipher, s2cCipher, false)
	server = NewTcpCrypto(id, s2cCipher, c2sCipher, true)
	return
}

func encryptBuf(t *testing.T, tc *TcpCrypto, msg []byte) []byte {
	t.Helper()
	// Reserve epochPrefixSize bytes at the start for the epoch tag.
	buf := make([]byte, epochPrefixSize+len(msg), epochPrefixSize+len(msg)+chacha20poly1305.Overhead)
	copy(buf[epochPrefixSize:], msg)
	ct, err := tc.Encrypt(buf)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	return ct
}

func TestTcpCrypto_RoundTrip(t *testing.T) {
	client, server := newCryptoPair(t)

	msg := []byte("hello epoch")
	ct := encryptBuf(t, client, msg)

	// Verify epoch prefix (epoch 0 for initial session).
	epoch := binary.BigEndian.Uint16(ct[:epochPrefixSize])
	if epoch != 0 {
		t.Fatalf("expected epoch 0, got %d", epoch)
	}

	// Total length = msg + poly1305 tag + 2-byte epoch.
	wantLen := len(msg) + chacha20poly1305.Overhead + epochPrefixSize
	if len(ct) != wantLen {
		t.Fatalf("ciphertext len=%d, want %d", len(ct), wantLen)
	}

	pt, err := server.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(pt, msg) {
		t.Fatalf("round-trip mismatch: got %q want %q", pt, msg)
	}
}

func TestTcpCrypto_BidirectionalRoundTrip(t *testing.T) {
	client, server := newCryptoPair(t)

	// Client → Server
	msg1 := []byte("client to server")
	ct1 := encryptBuf(t, client, msg1)
	pt1, err := server.Decrypt(ct1)
	if err != nil {
		t.Fatalf("Decrypt C2S: %v", err)
	}
	if !bytes.Equal(pt1, msg1) {
		t.Fatalf("C2S mismatch: got %q want %q", pt1, msg1)
	}

	// Server → Client
	msg2 := []byte("server to client")
	ct2 := encryptBuf(t, server, msg2)
	pt2, err := client.Decrypt(ct2)
	if err != nil {
		t.Fatalf("Decrypt S2C: %v", err)
	}
	if !bytes.Equal(pt2, msg2) {
		t.Fatalf("S2C mismatch: got %q want %q", pt2, msg2)
	}
}

func TestTcpCrypto_Rekey_DualEpoch(t *testing.T) {
	client, server := newCryptoPair(t)

	// Send a message with epoch 0.
	msg1 := []byte("before rekey")
	ct1 := encryptBuf(t, client, msg1)

	// Rekey both sides with new keys.
	newKeyC2S := randKey()
	newKeyS2C := randKey()

	// Server rekeys first (does NOT change send epoch).
	_, err := server.Rekey(newKeyS2C, newKeyC2S)
	if err != nil {
		t.Fatalf("server Rekey: %v", err)
	}

	// Client rekeys.
	clientEpoch, err := client.Rekey(newKeyC2S, newKeyS2C)
	if err != nil {
		t.Fatalf("client Rekey: %v", err)
	}

	// Server should still decrypt old-epoch frame (recv nonce for old session
	// hasn't been used yet, so it advances 0→1 matching ct1's nonce of 1).
	pt1, err := server.Decrypt(ct1)
	if err != nil {
		t.Fatalf("Decrypt old-epoch frame after rekey: %v", err)
	}
	if !bytes.Equal(pt1, msg1) {
		t.Fatalf("old-epoch mismatch: got %q want %q", pt1, msg1)
	}

	// Client switches send to new epoch and sends a new message.
	client.SetSendEpoch(clientEpoch)

	msg2 := []byte("after rekey")
	ct2 := encryptBuf(t, client, msg2)

	// Verify new epoch in the frame.
	epoch2 := binary.BigEndian.Uint16(ct2[:epochPrefixSize])
	if epoch2 != clientEpoch {
		t.Fatalf("expected epoch %d, got %d", clientEpoch, epoch2)
	}

	// Server decrypts new-epoch frame.
	pt2, err := server.Decrypt(ct2)
	if err != nil {
		t.Fatalf("Decrypt new-epoch frame: %v", err)
	}
	if !bytes.Equal(pt2, msg2) {
		t.Fatalf("new-epoch mismatch: got %q want %q", pt2, msg2)
	}
}

func TestTcpCrypto_Rekey_SendStillUsesOldEpoch(t *testing.T) {
	client, _ := newCryptoPair(t)

	newKey := randKey()
	_, err := client.Rekey(newKey, newKey)
	if err != nil {
		t.Fatalf("Rekey: %v", err)
	}

	// Encrypt should still use old epoch (0) because SetSendEpoch hasn't been called.
	msg := []byte("still old")
	ct := encryptBuf(t, client, msg)

	epoch := binary.BigEndian.Uint16(ct[:epochPrefixSize])
	if epoch != 0 {
		t.Fatalf("expected epoch 0 (old), got %d", epoch)
	}
}

func TestTcpCrypto_AutoCleanup_PrevClearedOnCurrentEpochDecrypt(t *testing.T) {
	client, server := newCryptoPair(t)

	// Rekey both sides.
	newKeyC2S := randKey()
	newKeyS2C := randKey()

	_, err := server.Rekey(newKeyS2C, newKeyC2S)
	if err != nil {
		t.Fatalf("server Rekey: %v", err)
	}
	clientEpoch, err := client.Rekey(newKeyC2S, newKeyS2C)
	if err != nil {
		t.Fatalf("client Rekey: %v", err)
	}
	client.SetSendEpoch(clientEpoch)

	// Server should have prev set.
	server.mu.RLock()
	hasPrev := server.prev != nil
	server.mu.RUnlock()
	if !hasPrev {
		t.Fatal("expected server prev to be set after Rekey")
	}

	// Client sends with new epoch → server decrypts → triggers auto-cleanup.
	msg := []byte("new-epoch-data")
	ct := encryptBuf(t, client, msg)
	if _, err := server.Decrypt(ct); err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	// Now server.prev should be nil.
	server.mu.RLock()
	hasPrev = server.prev != nil
	server.mu.RUnlock()
	if hasPrev {
		t.Fatal("expected server prev to be nil after current-epoch decrypt")
	}
}

func TestTcpCrypto_Encrypt_InsufficientCapacity(t *testing.T) {
	id := randID()
	key := randKey()
	aead, _ := chacha20poly1305.New(key)
	tc := NewTcpCrypto(id, aead, aead, false)

	msg := make([]byte, 32) // cap=32, need 32+16+2=50
	if _, err := tc.Encrypt(msg); err == nil {
		t.Fatal("expected error for insufficient capacity")
	}
}

func TestTcpCrypto_Decrypt_TooShort(t *testing.T) {
	id := randID()
	key := randKey()
	aead, _ := chacha20poly1305.New(key)
	tc := NewTcpCrypto(id, aead, aead, false)

	if _, err := tc.Decrypt([]byte{0x00}); err == nil {
		t.Fatal("expected error for frame too short")
	}
}

func TestTcpCrypto_Decrypt_UnknownEpoch(t *testing.T) {
	id := randID()
	key := randKey()
	aead, _ := chacha20poly1305.New(key)
	tc := NewTcpCrypto(id, aead, aead, false)

	data := make([]byte, 20)
	binary.BigEndian.PutUint16(data[:2], 99) // unknown epoch
	if _, err := tc.Decrypt(data); err == nil {
		t.Fatal("expected error for unknown epoch")
	}
}

func TestTcpCrypto_RemoveEpoch_NoOp(t *testing.T) {
	id := randID()
	key := randKey()
	aead, _ := chacha20poly1305.New(key)
	tc := NewTcpCrypto(id, aead, aead, false)

	if !tc.RemoveEpoch(0) {
		t.Fatal("RemoveEpoch should always return true for TCP")
	}
	if !tc.RemoveEpoch(42) {
		t.Fatal("RemoveEpoch should always return true for TCP")
	}
}

func TestTcpSession_Encrypt_NonceOverflow(t *testing.T) {
	id := randID()
	key := randKey()
	s, err := NewTcpCryptographyService(id, key, key, false)
	if err != nil {
		t.Fatalf("NewTcpCryptographyService: %v", err)
	}
	s.SendNonce.counterHigh = ^uint16(0)
	s.SendNonce.counterLow = ^uint64(0)

	msg := make([]byte, 1, 1+chacha20poly1305.Overhead)
	if _, err := s.Encrypt(msg); err == nil {
		t.Fatal("expected nonce overflow error")
	}
}

func TestTcpSession_Decrypt_PeekNonceOverflow(t *testing.T) {
	id := randID()
	key := randKey()
	s, err := NewTcpCryptographyService(id, key, key, false)
	if err != nil {
		t.Fatalf("NewTcpCryptographyService: %v", err)
	}
	s.RecvNonce.counterHigh = ^uint16(0)
	s.RecvNonce.counterLow = ^uint64(0)

	if _, err := s.Decrypt([]byte{1}); err == nil {
		t.Fatal("expected nonce overflow error from peekEncode")
	}
}

func TestTcpCrypto_Encrypt_BufferTooShortForEpochPrefix(t *testing.T) {
	client, _ := newCryptoPair(t)
	if _, err := client.Encrypt([]byte{1}); err == nil {
		t.Fatal("expected buffer-too-short-for-prefix error")
	}
}

func TestTcpCrypto_Encrypt_PropagatesSessionEncryptError(t *testing.T) {
	client, _ := newCryptoPair(t)
	client.current.SendNonce.counterHigh = ^uint16(0)
	client.current.SendNonce.counterLow = ^uint64(0)

	buf := make([]byte, epochPrefixSize+1, epochPrefixSize+1+chacha20poly1305.Overhead)
	if _, err := client.Encrypt(buf); err == nil {
		t.Fatal("expected session encrypt error")
	}
}

func TestTcpCrypto_Decrypt_PropagatesSessionDecryptError(t *testing.T) {
	_, server := newCryptoPair(t)
	// Known epoch=0 but random payload should fail authentication in session decrypt.
	frame := make([]byte, epochPrefixSize+chacha20poly1305.Overhead+1)
	binary.BigEndian.PutUint16(frame[:epochPrefixSize], 0)
	if _, err := server.Decrypt(frame); err == nil {
		t.Fatal("expected decrypt failure for malformed ciphertext")
	}
}

func TestTcpCrypto_Rekey_BadRecvKey(t *testing.T) {
	client, _ := newCryptoPair(t)
	good := randKey()
	bad := []byte("short")

	if _, err := client.Rekey(good, bad); err == nil {
		t.Fatal("expected rekey error for invalid recv key")
	}
}

func TestTcpCrypto_Rekey_BadSendKey(t *testing.T) {
	client, _ := newCryptoPair(t)
	if _, err := client.Rekey([]byte("short"), randKey()); err == nil {
		t.Fatal("expected rekey error for invalid send key")
	}
}

func TestTcpCrypto_Zeroize(t *testing.T) {
	client, _ := newCryptoPair(t)
	newKey := randKey()
	if _, err := client.Rekey(newKey, newKey); err != nil {
		t.Fatalf("Rekey: %v", err)
	}
	if client.prev == nil {
		t.Fatal("expected prev session after rekey")
	}

	client.Zeroize()

	if client.sessionId != [32]byte{} {
		t.Fatal("expected session id to be zeroized")
	}
	if client.current.SendNonce.counterLow != 0 || client.current.SendNonce.counterHigh != 0 {
		t.Fatal("expected current send nonce to be zeroized")
	}
	if client.prev.SendNonce.counterLow != 0 || client.prev.SendNonce.counterHigh != 0 {
		t.Fatal("expected prev send nonce to be zeroized")
	}
}
