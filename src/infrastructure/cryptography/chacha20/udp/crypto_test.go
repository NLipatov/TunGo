package udp

import (
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"testing"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/internal/core"

	"golang.org/x/crypto/chacha20poly1305"
)

func TestCrypto_StageEpoch_AllowsSafetyLimitEpoch(t *testing.T) {
	crypto := makeCrypto(t)
	crypto.epochCounter = core.MaxEpoch - 1
	crypto.ring = &testEpochRing{current: core.MaxEpoch - 1}
	key := make([]byte, chacha20poly1305.KeySize)

	epoch, err := crypto.StageEpoch(key, key)
	if err != nil {
		t.Fatalf("Rekey failed at safety limit: %v", err)
	}
	if epoch != core.MaxEpoch {
		t.Fatalf("expected epoch %d, got %d", core.MaxEpoch, epoch)
	}
}

func TestCrypto_StageEpoch_RejectsEpochPastSafetyLimit(t *testing.T) {
	crypto := makeCrypto(t)
	crypto.epochCounter = core.MaxEpoch
	crypto.ring = &testEpochRing{current: core.MaxEpoch}
	key := make([]byte, chacha20poly1305.KeySize)

	_, err := crypto.StageEpoch(key, key)
	if !errors.Is(err, chacha20.ErrEpochExhausted) {
		t.Fatalf("expected ErrEpochExhausted, got %v", err)
	}
	if crypto.ring.Current() != core.MaxEpoch {
		t.Fatalf("current epoch changed after rejection: %d", crypto.ring.Current())
	}
}

type testEpochRing struct {
	current        uint16
	lenVal         int
	capVal         int
	oldest         uint16
	hasOldest      bool
	resolveSession *Session
	resolveOK      bool
	removeResult   bool
	zeroized       bool
}

func (r *testEpochRing) Current() uint16 { return r.current }
func (r *testEpochRing) Resolve(_ uint16) (*Session, bool) {
	return r.resolveSession, r.resolveOK
}
func (r *testEpochRing) Insert(_ uint16, _ *Session) {}
func (r *testEpochRing) ResolveCurrent() (*Session, bool) {
	return r.resolveSession, r.resolveOK
}
func (r *testEpochRing) Oldest() (uint16, bool) { return r.oldest, r.hasOldest }
func (r *testEpochRing) Len() int               { return r.lenVal }
func (r *testEpochRing) Capacity() int          { return r.capVal }
func (r *testEpochRing) Remove(_ uint16) bool   { return r.removeResult }
func (r *testEpochRing) ZeroizeAll()            { r.zeroized = true }

type badAEAD struct{}

func (badAEAD) NonceSize() int { return chacha20poly1305.NonceSize }
func (badAEAD) Overhead() int  { return chacha20poly1305.Overhead }
func (badAEAD) Seal(dst, nonce, plaintext, additionalData []byte) []byte {
	_ = nonce
	_ = additionalData
	out := make([]byte, len(dst)+len(plaintext))
	copy(out, dst)
	copy(out[len(dst):], plaintext)
	return out
}
func (badAEAD) Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	_ = nonce
	_ = ciphertext
	_ = additionalData
	return dst, nil
}

func newSessionWithAEAD(epoch uint16, a cipher.AEAD) *Session {
	return newSessionWithCiphers([32]byte{}, a, a, false, epoch)
}

func makeCrypto(t *testing.T) *Crypto {
	t.Helper()
	key := make([]byte, chacha20poly1305.KeySize)
	sendCipher, err := chacha20poly1305.New(key)
	if err != nil {
		t.Fatal(err)
	}
	recvCipher, err := chacha20poly1305.New(key)
	if err != nil {
		t.Fatal(err)
	}
	return NewCrypto([32]byte{}, sendCipher, recvCipher, false)
}

func makeCryptoPair(t *testing.T) (client, server *Crypto) {
	t.Helper()
	key := make([]byte, chacha20poly1305.KeySize)
	// Client: sendCipher=C2S key, recvCipher=S2C key.
	// Server: sendCipher=S2C key, recvCipher=C2S key.
	// For simplicity use same key for both; direction AAD differs.
	c2sCipher1, _ := chacha20poly1305.New(key)
	s2cCipher1, _ := chacha20poly1305.New(key)
	c2sCipher2, _ := chacha20poly1305.New(key)
	s2cCipher2, _ := chacha20poly1305.New(key)
	client = NewCrypto([32]byte{}, c2sCipher1, s2cCipher1, false)
	server = NewCrypto([32]byte{}, s2cCipher2, c2sCipher2, true)
	return client, server
}

func TestCrypto_EncryptDecrypt_RoundTrip(t *testing.T) {
	client, server := makeCryptoPair(t)
	payload := []byte("hello world")

	// Client encrypts → server decrypts.
	buf := make(
		[]byte,
		RouteIDLength+chacha20poly1305.NonceSize+len(payload),
		RouteIDLength+chacha20poly1305.NonceSize+len(payload)+chacha20poly1305.Overhead,
	)
	copy(buf[RouteIDLength+chacha20poly1305.NonceSize:], payload)

	encrypted, err := client.Encrypt(buf)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := server.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != string(payload) {
		t.Fatalf("expected %q, got %q", payload, decrypted)
	}
}

func TestCrypto_StageEpoch_UsesCanonicalDirections(t *testing.T) {
	client, server := makeCryptoPair(t)
	c2s := randKey()
	s2c := randKey()

	clientEpoch, err := client.StageEpoch(c2s, s2c)
	if err != nil {
		t.Fatalf("client StageEpoch: %v", err)
	}
	serverEpoch, err := server.StageEpoch(c2s, s2c)
	if err != nil {
		t.Fatalf("server StageEpoch: %v", err)
	}
	client.PromoteSendEpoch(clientEpoch)
	server.PromoteSendEpoch(serverEpoch)

	roundTrip := func(sender, receiver *Crypto, payload []byte) {
		t.Helper()
		buf := make(
			[]byte,
			RouteIDLength+chacha20poly1305.NonceSize+len(payload),
			RouteIDLength+chacha20poly1305.NonceSize+len(payload)+chacha20poly1305.Overhead,
		)
		copy(buf[RouteIDLength+chacha20poly1305.NonceSize:], payload)
		encrypted, err := sender.Encrypt(buf)
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		decrypted, err := receiver.Decrypt(encrypted)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if string(decrypted) != string(payload) {
			t.Fatalf("decrypted %q, want %q", decrypted, payload)
		}
	}

	roundTrip(client, server, []byte("c2s"))
	roundTrip(server, client, []byte("s2c"))
}

func TestCrypto_Decrypt_TooShort(t *testing.T) {
	c := makeCrypto(t)
	_, err := c.Decrypt(make([]byte, MinPacketSize-1))
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}

func TestCrypto_Decrypt_UnknownEpoch(t *testing.T) {
	c := makeCrypto(t)

	// Craft a packet with matching route-id and unknown epoch.
	buf := make([]byte, MinPacketSize)
	binary.BigEndian.PutUint16(buf[EpochOffset:EpochOffset+2], 99)

	_, err := c.Decrypt(buf)
	if !errors.Is(err, ErrUnknownEpoch) {
		t.Fatalf("expected ErrUnknownEpoch, got %v", err)
	}
}

func TestCrypto_Encrypt_NoActiveSession(t *testing.T) {
	// Create crypto, then manipulate ring to have no sessions.
	c := makeCrypto(t)
	c.ring.Remove(0)
	c.PromoteSendEpoch(99)

	buf := make(
		[]byte,
		RouteIDLength+chacha20poly1305.NonceSize+10,
		RouteIDLength+chacha20poly1305.NonceSize+10+chacha20poly1305.Overhead,
	)
	_, err := c.Encrypt(buf)
	if err == nil {
		t.Fatal("expected error when no active session")
	}
}

func TestCrypto_StageEpoch_StagesNewEpoch(t *testing.T) {
	c := makeCrypto(t)
	key := make([]byte, chacha20poly1305.KeySize)

	epoch, err := c.StageEpoch(key, key)
	if err != nil {
		t.Fatalf("Rekey failed: %v", err)
	}
	if epoch != 1 {
		t.Fatalf("expected epoch=1, got %d", epoch)
	}
	if c.ring.Len() != 2 {
		t.Fatalf("expected 2 sessions in ring, got %d", c.ring.Len())
	}

	// Verify new epoch is resolvable.
	s, ok := c.ring.Resolve(epoch)
	if !ok {
		t.Fatal("expected new epoch to be resolvable")
	}
	if s.Epoch() != epoch {
		t.Fatalf("expected session epoch=%d, got %d", epoch, s.Epoch())
	}
}

func TestCrypto_StageEpoch_RefusesWhenSendEpochWouldBeEvicted(t *testing.T) {
	c := makeCrypto(t)
	key := make([]byte, chacha20poly1305.KeySize)

	// Fill ring to capacity (default=4).
	for i := 0; i < 3; i++ {
		if _, err := c.StageEpoch(key, key); err != nil {
			t.Fatalf("Rekey %d failed: %v", i, err)
		}
	}
	// ring is full: epochs 0,1,2,3. sendEpoch=0 is oldest.
	if c.ring.Len() != c.ring.Capacity() {
		t.Fatalf("expected ring at capacity, got %d/%d", c.ring.Len(), c.ring.Capacity())
	}

	// Next rekey would evict epoch 0 which is still the send epoch.
	_, err := c.StageEpoch(key, key)
	if err == nil {
		t.Fatal("expected error when send epoch would be evicted")
	}
}

func TestCrypto_PromoteSendEpoch(t *testing.T) {
	c := makeCrypto(t)
	c.PromoteSendEpoch(42)
	if c.currentSendEpoch() != 42 {
		t.Fatalf("expected sendEpoch=42, got %d", c.currentSendEpoch())
	}
}

func TestCrypto_StageEpoch_BadKey(t *testing.T) {
	c := makeCrypto(t)
	_, err := c.StageEpoch([]byte("short"), []byte("short"))
	if err == nil {
		t.Fatal("expected error for invalid key length")
	}
}

func TestCrypto_Decrypt_EpochMismatch(t *testing.T) {
	c := makeCrypto(t)
	r := &testEpochRing{
		resolveSession: newSessionWithAEAD(7, badAEAD{}),
		resolveOK:      true,
	}
	c.ring = r

	buf := make([]byte, MinPacketSize)
	binary.BigEndian.PutUint16(buf[EpochOffset:EpochOffset+2], 0)

	_, err := c.Decrypt(buf)
	if !errors.Is(err, ErrUnknownEpoch) {
		t.Fatalf("expected ErrUnknownEpoch on epoch mismatch, got %v", err)
	}
}

func TestCrypto_StageEpoch_BadS2CKey(t *testing.T) {
	c := makeCrypto(t)
	_, err := c.StageEpoch(make([]byte, chacha20poly1305.KeySize), []byte("short"))
	if err == nil {
		t.Fatal("expected rekey S2C key error")
	}
}

func TestCrypto_RetirePreviousEpoch_DefersToRingEviction(t *testing.T) {
	c := makeCrypto(t)
	if !c.RetirePreviousEpoch() {
		t.Fatal("expected retirement notification to be accepted")
	}
	if c.ring.Len() != 1 {
		t.Fatalf("expected UDP ring to retain old session, got %d entries", c.ring.Len())
	}
}

func TestCrypto_Zeroize(t *testing.T) {
	c := makeCrypto(t)
	r := &testEpochRing{}
	c.ring = r
	for i := range c.sessionId {
		c.sessionId[i] = byte(i + 1)
	}

	c.Zeroize()

	if !r.zeroized {
		t.Fatal("expected ring.ZeroizeAll to be called")
	}
	if c.sessionId != [32]byte{} {
		t.Fatal("expected sessionId to be zeroized")
	}
}

func TestCrypto_Decrypt_UnknownRouteID(t *testing.T) {
	var sessionID [32]byte
	copy(sessionID[:8], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	key := make([]byte, chacha20poly1305.KeySize)
	sendCipher, _ := chacha20poly1305.New(key)
	recvCipher, _ := chacha20poly1305.New(key)
	c := NewCrypto(sessionID, sendCipher, recvCipher, false)

	packet := make([]byte, MinPacketSize)
	binary.BigEndian.PutUint64(packet[:RouteIDLength], 0x9988776655443322)

	_, err := c.Decrypt(packet)
	if !errors.Is(err, ErrUnknownRouteID) {
		t.Fatalf("expected ErrUnknownRouteID, got %v", err)
	}
}

func TestCrypto_RouteID(t *testing.T) {
	var sessionID [32]byte
	copy(sessionID[:8], []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11})
	key := make([]byte, chacha20poly1305.KeySize)
	sendCipher, _ := chacha20poly1305.New(key)
	recvCipher, _ := chacha20poly1305.New(key)
	c := NewCrypto(sessionID, sendCipher, recvCipher, false)

	if got := c.RouteID(); got != RouteIDFromSessionID(sessionID) {
		t.Fatalf("unexpected route id: got %x", got)
	}
}
