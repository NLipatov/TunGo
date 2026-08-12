package tcp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
	"tungo/internal/protocol/chacha20"
	"tungo/internal/protocol/chacha20/internal/core"
	"tungo/internal/protocol/chacha20/rekey"

	"golang.org/x/crypto/chacha20poly1305"
)

func TestCrypto_StageEpoch_AllowsSafetyLimitEpoch(t *testing.T) {
	client, _ := newCryptoPair(t)
	client.epochCounter = core.MaxEpoch - 1

	epoch, err := client.StageEpoch(randKey(), randKey())
	if err != nil {
		t.Fatalf("Rekey failed at safety limit: %v", err)
	}
	if epoch != core.MaxEpoch {
		t.Fatalf("expected epoch %d, got %d", core.MaxEpoch, epoch)
	}
}

func TestCrypto_StageEpoch_RejectsEpochPastSafetyLimit(t *testing.T) {
	client, _ := newCryptoPair(t)
	client.epochCounter = core.MaxEpoch
	newest := client.recvNewest

	_, err := client.StageEpoch(randKey(), randKey())
	if !errors.Is(err, chacha20.ErrEpochExhausted) {
		t.Fatalf("expected ErrEpochExhausted, got %v", err)
	}
	if client.epochCounter != core.MaxEpoch {
		t.Fatalf("epoch counter changed after rejection: %d", client.epochCounter)
	}
	if client.recvNewest != newest {
		t.Fatal("newest receive slot changed after rejected rekey")
	}
}

func newCryptoPair(t *testing.T) (client, server *Crypto) {
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

	client = NewCrypto(id, c2sCipher, s2cCipher, false)
	server = NewCrypto(id, s2cCipher, c2sCipher, true)
	return
}

func encryptBuf(t *testing.T, tc *Crypto, msg []byte) []byte {
	t.Helper()
	// Reserve EpochPrefixSize bytes at the start for the epoch tag.
	buf := make([]byte, EpochPrefixSize+len(msg), EpochPrefixSize+len(msg)+chacha20poly1305.Overhead)
	copy(buf[EpochPrefixSize:], msg)
	ct, err := tc.Encrypt(buf)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	return ct
}

func TestCrypto_RoundTrip(t *testing.T) {
	client, server := newCryptoPair(t)

	msg := []byte("hello epoch")
	ct := encryptBuf(t, client, msg)

	// Verify epoch prefix (epoch 0 for initial session).
	epoch := binary.BigEndian.Uint16(ct[:EpochPrefixSize])
	if epoch != 0 {
		t.Fatalf("expected epoch 0, got %d", epoch)
	}

	// Total length = msg + poly1305 tag + 2-byte epoch.
	wantLen := len(msg) + chacha20poly1305.Overhead + EpochPrefixSize
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

func TestCrypto_BidirectionalRoundTrip(t *testing.T) {
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

func TestCrypto_StageEpoch_DualEpoch(t *testing.T) {
	client, server := newCryptoPair(t)

	// Send a message with epoch 0.
	msg1 := []byte("before rekey")
	ct1 := encryptBuf(t, client, msg1)

	// Rekey both sides with new keys.
	newKeyC2S := randKey()
	newKeyS2C := randKey()

	// Server rekeys first (does NOT change send epoch).
	_, err := server.StageEpoch(newKeyC2S, newKeyS2C)
	if err != nil {
		t.Fatalf("server Rekey: %v", err)
	}

	// Client rekeys.
	clientEpoch, err := client.StageEpoch(newKeyC2S, newKeyS2C)
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
	client.PromoteSendEpoch(clientEpoch)

	msg2 := []byte("after rekey")
	ct2 := encryptBuf(t, client, msg2)

	// Verify new epoch in the frame.
	epoch2 := binary.BigEndian.Uint16(ct2[:EpochPrefixSize])
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

func TestCrypto_StageEpoch_SendStillUsesOldEpoch(t *testing.T) {
	client, _ := newCryptoPair(t)

	newKey := randKey()
	_, err := client.StageEpoch(newKey, newKey)
	if err != nil {
		t.Fatalf("Rekey: %v", err)
	}

	// Encrypt should still use old epoch (0) because PromoteSendEpoch hasn't been called.
	msg := []byte("still old")
	ct := encryptBuf(t, client, msg)

	epoch := binary.BigEndian.Uint16(ct[:EpochPrefixSize])
	if epoch != 0 {
		t.Fatalf("expected epoch 0 (old), got %d", epoch)
	}
	client.mu.RLock()
	sendIsPrevious := client.send == client.recvPrevious
	client.mu.RUnlock()
	if !sendIsPrevious {
		t.Fatal("expected send slot to remain the previous receive slot until activation")
	}
}

func TestCrypto_PromoteSendEpoch_UnknownEpochKeepsConsistentSlot(t *testing.T) {
	client, server := newCryptoPair(t)
	if _, err := client.StageEpoch(randKey(), randKey()); err != nil {
		t.Fatalf("Rekey: %v", err)
	}

	client.PromoteSendEpoch(99)

	client.mu.RLock()
	send := client.send
	previous := client.recvPrevious
	client.mu.RUnlock()
	if send != previous {
		t.Fatalf("unknown epoch changed send slot: send=%+v previous=%+v", send, previous)
	}

	ct := encryptBuf(t, client, []byte("known slot"))
	if epoch := binary.BigEndian.Uint16(ct[:EpochPrefixSize]); epoch != send.epoch {
		t.Fatalf("wire epoch %d does not match send slot epoch %d", epoch, send.epoch)
	}
	if _, err := server.Decrypt(ct); err != nil {
		t.Fatalf("peer could not decrypt packet from retained send slot: %v", err)
	}
}

func TestCrypto_PromoteSendEpoch_PreviousEpochDoesNotRollback(t *testing.T) {
	client, _ := newCryptoPair(t)
	newestEpoch, err := client.StageEpoch(randKey(), randKey())
	if err != nil {
		t.Fatalf("StageEpoch: %v", err)
	}
	client.PromoteSendEpoch(newestEpoch)

	client.PromoteSendEpoch(0)

	client.mu.RLock()
	defer client.mu.RUnlock()
	if client.send != client.recvNewest {
		t.Fatal("previous epoch rolled send session back from the newest epoch")
	}
}

func TestCrypto_StageEpochRefusesToOverwriteUnretiredPreviousEpoch(t *testing.T) {
	client, _ := newCryptoPair(t)
	epoch, err := client.StageEpoch(randKey(), randKey())
	if err != nil {
		t.Fatalf("first Rekey: %v", err)
	}
	client.PromoteSendEpoch(epoch)

	if _, err := client.StageEpoch(randKey(), randKey()); err == nil {
		t.Fatal("expected rekey to fail while previous receive epoch is still retained")
	}
}

func TestCrypto_DecryptDoesNotRetirePreviousEpoch(t *testing.T) {
	client, server := newCryptoPair(t)

	// Rekey both sides.
	newKeyC2S := randKey()
	newKeyS2C := randKey()

	_, err := server.StageEpoch(newKeyC2S, newKeyS2C)
	if err != nil {
		t.Fatalf("server Rekey: %v", err)
	}
	clientEpoch, err := client.StageEpoch(newKeyC2S, newKeyS2C)
	if err != nil {
		t.Fatalf("client Rekey: %v", err)
	}
	client.PromoteSendEpoch(clientEpoch)

	// Server should retain the previous receive slot.
	server.mu.RLock()
	hasPrev := server.recvPrevious.session != nil
	server.mu.RUnlock()
	if !hasPrev {
		t.Fatal("expected previous receive slot after Rekey")
	}

	// Client sends with new epoch and server authenticates it.
	msg := []byte("new-epoch-data")
	ct := encryptBuf(t, client, msg)
	if _, err := server.Decrypt(ct); err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	// Decrypt reports only cryptographic success; lifecycle policy keeps prev.
	server.mu.RLock()
	hasPrev = server.recvPrevious.session != nil
	server.mu.RUnlock()
	if !hasPrev {
		t.Fatal("expected Decrypt to leave previous epoch untouched")
	}

	// Retirement is explicit and cannot happen while send still uses prev.
	if server.RetirePreviousEpoch() {
		t.Fatal("expected retirement to fail while send still uses epoch 0")
	}
	server.PromoteSendEpoch(clientEpoch)
	if !server.RetirePreviousEpoch() {
		t.Fatal("expected explicit retirement of epoch 0 to succeed")
	}
	server.mu.RLock()
	hasPrev = server.recvPrevious.session != nil
	server.mu.RUnlock()
	if hasPrev {
		t.Fatal("expected previous epoch to be cleared after explicit retirement")
	}
}

func TestCrypto_Encrypt_InsufficientCapacity(t *testing.T) {
	id := randID()
	key := randKey()
	aead, _ := chacha20poly1305.New(key)
	tc := NewCrypto(id, aead, aead, false)

	msg := make([]byte, 32) // cap=32, need 32+16+2=50
	if _, err := tc.Encrypt(msg); err == nil {
		t.Fatal("expected error for insufficient capacity")
	}
}

func TestCrypto_Decrypt_TooShort(t *testing.T) {
	id := randID()
	key := randKey()
	aead, _ := chacha20poly1305.New(key)
	tc := NewCrypto(id, aead, aead, false)

	if _, err := tc.Decrypt([]byte{0x00}); err == nil {
		t.Fatal("expected error for frame too short")
	}
}

func TestCrypto_Decrypt_UnknownEpoch(t *testing.T) {
	id := randID()
	key := randKey()
	aead, _ := chacha20poly1305.New(key)
	tc := NewCrypto(id, aead, aead, false)

	data := make([]byte, 20)
	binary.BigEndian.PutUint16(data[:2], 99) // unknown epoch
	if _, err := tc.Decrypt(data); err == nil {
		t.Fatal("expected error for unknown epoch")
	}
}

func TestCrypto_FSMRetiresPreviousEpochBeforeNextRekey(t *testing.T) {
	client, server := newCryptoPair(t)
	clientFSM := rekey.NewStateMachine(client, []byte("old-c2s"), []byte("old-s2c"))
	serverFSM := rekey.NewStateMachine(server, []byte("old-c2s"), []byte("old-s2c"))
	newC2S := randKey()
	newS2C := randKey()

	clientEpoch, err := clientFSM.StartRekey(newC2S, newS2C)
	if err != nil {
		t.Fatalf("client StartRekey: %v", err)
	}
	serverEpoch, err := serverFSM.StartRekey(newC2S, newS2C)
	if err != nil {
		t.Fatalf("server StartRekey: %v", err)
	}
	clientFSM.ActivateSendEpoch(clientEpoch)
	serverFSM.ActivateSendEpoch(serverEpoch)

	clientFrame := encryptBuf(t, client, []byte("client current epoch"))
	if _, err := server.Decrypt(clientFrame); err != nil {
		t.Fatalf("server Decrypt: %v", err)
	}
	serverFSM.ObservePeerEpoch(binary.BigEndian.Uint16(clientFrame[:EpochPrefixSize]))

	serverFrame := encryptBuf(t, server, []byte("server current epoch"))
	if _, err := client.Decrypt(serverFrame); err != nil {
		t.Fatalf("client Decrypt: %v", err)
	}
	clientFSM.ObservePeerEpoch(binary.BigEndian.Uint16(serverFrame[:EpochPrefixSize]))

	client.mu.RLock()
	clientHasPrev := client.recvPrevious.session != nil
	client.mu.RUnlock()
	server.mu.RLock()
	serverHasPrev := server.recvPrevious.session != nil
	server.mu.RUnlock()
	if clientHasPrev || serverHasPrev {
		t.Fatalf("expected both previous epochs retired, client=%v server=%v", clientHasPrev, serverHasPrev)
	}

	if _, err := clientFSM.StartRekey(randKey(), randKey()); err != nil {
		t.Fatalf("next client StartRekey: %v", err)
	}
	if _, err := serverFSM.StartRekey(randKey(), randKey()); err != nil {
		t.Fatalf("next server StartRekey: %v", err)
	}
}

func TestCrypto_Encrypt_BufferTooShortForEpochPrefix(t *testing.T) {
	client, _ := newCryptoPair(t)
	if _, err := client.Encrypt([]byte{1}); err == nil {
		t.Fatal("expected buffer-too-short-for-prefix error")
	}
}

func TestCrypto_Encrypt_PropagatesSessionEncryptError(t *testing.T) {
	client, _ := newCryptoPair(t)
	client.recvNewest.session.SendNonce.CounterHigh = ^uint16(0)
	client.recvNewest.session.SendNonce.CounterLow = ^uint64(0)

	buf := make([]byte, EpochPrefixSize+1, EpochPrefixSize+1+chacha20poly1305.Overhead)
	if _, err := client.Encrypt(buf); err == nil {
		t.Fatal("expected session encrypt error")
	}
}

func TestCrypto_Decrypt_PropagatesSessionDecryptError(t *testing.T) {
	_, server := newCryptoPair(t)
	// Known epoch=0 but random payload should fail authentication in session decrypt.
	frame := make([]byte, EpochPrefixSize+chacha20poly1305.Overhead+1)
	binary.BigEndian.PutUint16(frame[:EpochPrefixSize], 0)
	if _, err := server.Decrypt(frame); err == nil {
		t.Fatal("expected decrypt failure for malformed ciphertext")
	}
}

func TestCrypto_StageEpoch_BadS2CKey(t *testing.T) {
	client, _ := newCryptoPair(t)
	good := randKey()
	bad := []byte("short")

	if _, err := client.StageEpoch(good, bad); err == nil {
		t.Fatal("expected rekey error for invalid S2C key")
	}
}

func TestCrypto_StageEpoch_BadC2SKey(t *testing.T) {
	client, _ := newCryptoPair(t)
	if _, err := client.StageEpoch([]byte("short"), randKey()); err == nil {
		t.Fatal("expected rekey error for invalid C2S key")
	}
}

func TestCrypto_Zeroize(t *testing.T) {
	client, _ := newCryptoPair(t)
	newKey := randKey()
	if _, err := client.StageEpoch(newKey, newKey); err != nil {
		t.Fatalf("Rekey: %v", err)
	}
	if client.recvPrevious.session == nil {
		t.Fatal("expected previous receive session after rekey")
	}

	client.Zeroize()

	if client.sessionId != [32]byte{} {
		t.Fatal("expected session id to be zeroized")
	}
	if client.recvNewest.session.SendNonce.CounterLow != 0 || client.recvNewest.session.SendNonce.CounterHigh != 0 {
		t.Fatal("expected newest session send nonce to be zeroized")
	}
	if client.recvPrevious.session.SendNonce.CounterLow != 0 || client.recvPrevious.session.SendNonce.CounterHigh != 0 {
		t.Fatal("expected previous session send nonce to be zeroized")
	}
}
