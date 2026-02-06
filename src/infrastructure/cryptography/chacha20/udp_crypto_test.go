package chacha20

import (
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

type testEpochRing struct {
	current        Epoch
	lenVal         int
	capVal         int
	oldest         Epoch
	hasOldest      bool
	resolveSession *DefaultUdpSession
	resolveOK      bool
	removeResult   bool
	zeroized       bool
}

func (r *testEpochRing) Current() Epoch { return r.current }
func (r *testEpochRing) Resolve(_ Epoch) (*DefaultUdpSession, bool) {
	return r.resolveSession, r.resolveOK
}
func (r *testEpochRing) Insert(_ Epoch, _ *DefaultUdpSession) {}
func (r *testEpochRing) ResolveCurrent() (*DefaultUdpSession, bool) {
	return r.resolveSession, r.resolveOK
}
func (r *testEpochRing) Oldest() (Epoch, bool) { return r.oldest, r.hasOldest }
func (r *testEpochRing) Len() int              { return r.lenVal }
func (r *testEpochRing) Capacity() int         { return r.capVal }
func (r *testEpochRing) Remove(_ Epoch) bool   { return r.removeResult }
func (r *testEpochRing) ZeroizeAll()           { r.zeroized = true }

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

func newUdpSessionWithAEAD(epoch Epoch, a cipher.AEAD) *DefaultUdpSession {
	return NewUdpSessionWithCiphers([32]byte{}, a, a, false, epoch)
}

func makeUdpCrypto(t *testing.T) *EpochUdpCrypto {
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
	return NewEpochUdpCrypto([32]byte{}, sendCipher, recvCipher, false)
}

func makeUdpCryptoPair(t *testing.T) (client, server *EpochUdpCrypto) {
	t.Helper()
	key := make([]byte, chacha20poly1305.KeySize)
	// Client: sendCipher=C2S key, recvCipher=S2C key.
	// Server: sendCipher=S2C key, recvCipher=C2S key.
	// For simplicity use same key for both; direction AAD differs.
	c2sCipher1, _ := chacha20poly1305.New(key)
	s2cCipher1, _ := chacha20poly1305.New(key)
	c2sCipher2, _ := chacha20poly1305.New(key)
	s2cCipher2, _ := chacha20poly1305.New(key)
	client = NewEpochUdpCrypto([32]byte{}, c2sCipher1, s2cCipher1, false)
	server = NewEpochUdpCrypto([32]byte{}, s2cCipher2, c2sCipher2, true)
	return client, server
}

func TestEpochUdpCrypto_EncryptDecrypt_RoundTrip(t *testing.T) {
	client, server := makeUdpCryptoPair(t)
	payload := []byte("hello world")

	// Client encrypts → server decrypts.
	buf := make([]byte, chacha20poly1305.NonceSize+len(payload), chacha20poly1305.NonceSize+len(payload)+chacha20poly1305.Overhead)
	copy(buf[chacha20poly1305.NonceSize:], payload)

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

func TestEpochUdpCrypto_Decrypt_TooShort(t *testing.T) {
	c := makeUdpCrypto(t)
	_, err := c.Decrypt(make([]byte, chacha20poly1305.NonceSize-1))
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}

func TestEpochUdpCrypto_Decrypt_UnknownEpoch(t *testing.T) {
	c := makeUdpCrypto(t)

	// Craft a packet with epoch=99 in nonce bytes 10-11.
	buf := make([]byte, chacha20poly1305.NonceSize+chacha20poly1305.Overhead+1)
	binary.BigEndian.PutUint16(buf[NonceEpochOffset:NonceEpochOffset+2], 99)

	_, err := c.Decrypt(buf)
	if !errors.Is(err, ErrUnknownEpoch) {
		t.Fatalf("expected ErrUnknownEpoch, got %v", err)
	}
}

func TestEpochUdpCrypto_Encrypt_NoActiveSession(t *testing.T) {
	// Create crypto, then manipulate ring to have no sessions.
	c := makeUdpCrypto(t)
	c.ring.Remove(0)
	c.SetSendEpoch(99)

	buf := make([]byte, chacha20poly1305.NonceSize+10, chacha20poly1305.NonceSize+10+chacha20poly1305.Overhead)
	_, err := c.Encrypt(buf)
	if err == nil {
		t.Fatal("expected error when no active session")
	}
}

func TestEpochUdpCrypto_Rekey_InstallsNewEpoch(t *testing.T) {
	c := makeUdpCrypto(t)
	key := make([]byte, chacha20poly1305.KeySize)

	epoch, err := c.Rekey(key, key)
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
	s, ok := c.ring.Resolve(Epoch(epoch))
	if !ok {
		t.Fatal("expected new epoch to be resolvable")
	}
	if s.Epoch() != Epoch(epoch) {
		t.Fatalf("expected session epoch=%d, got %d", epoch, s.Epoch())
	}
}

func TestEpochUdpCrypto_Rekey_RefusesWhenSendEpochWouldBeEvicted(t *testing.T) {
	c := makeUdpCrypto(t)
	key := make([]byte, chacha20poly1305.KeySize)

	// Fill ring to capacity (default=4).
	for i := 0; i < 3; i++ {
		if _, err := c.Rekey(key, key); err != nil {
			t.Fatalf("Rekey %d failed: %v", i, err)
		}
	}
	// ring is full: epochs 0,1,2,3. sendEpoch=0 is oldest.
	if c.ring.Len() != c.ring.Capacity() {
		t.Fatalf("expected ring at capacity, got %d/%d", c.ring.Len(), c.ring.Capacity())
	}

	// Next rekey would evict epoch 0 which is still the send epoch.
	_, err := c.Rekey(key, key)
	if err == nil {
		t.Fatal("expected error when send epoch would be evicted")
	}
}

func TestEpochUdpCrypto_SetSendEpoch(t *testing.T) {
	c := makeUdpCrypto(t)
	c.SetSendEpoch(42)
	if c.currentSendEpoch() != 42 {
		t.Fatalf("expected sendEpoch=42, got %d", c.currentSendEpoch())
	}
}

func TestEpochUdpCrypto_RemoveEpoch_CannotRemoveSendEpoch(t *testing.T) {
	c := makeUdpCrypto(t)
	key := make([]byte, chacha20poly1305.KeySize)
	c.Rekey(key, key)

	// sendEpoch is 0. Cannot remove it.
	if c.RemoveEpoch(0) {
		t.Fatal("expected RemoveEpoch to refuse removing active send epoch")
	}
}

func TestEpochUdpCrypto_RemoveEpoch_CannotRemoveLastEntry(t *testing.T) {
	c := makeUdpCrypto(t)
	// Only 1 entry in ring.
	if c.RemoveEpoch(0) {
		t.Fatal("expected RemoveEpoch to refuse removing last entry")
	}
}

func TestEpochUdpCrypto_RemoveEpoch_CannotRemoveLastEntry_NonSendEpoch(t *testing.T) {
	c := makeUdpCrypto(t)
	c.SetSendEpoch(99) // ensure first guard does not trigger
	if c.RemoveEpoch(0) {
		t.Fatal("expected RemoveEpoch to refuse removing last entry")
	}
}

func TestEpochUdpCrypto_RemoveEpoch_Success(t *testing.T) {
	c := makeUdpCrypto(t)
	key := make([]byte, chacha20poly1305.KeySize)
	epoch, _ := c.Rekey(key, key)
	c.SetSendEpoch(epoch)

	// Now we can remove epoch 0.
	if !c.RemoveEpoch(0) {
		t.Fatal("expected RemoveEpoch(0) to succeed")
	}
	if c.ring.Len() != 1 {
		t.Fatalf("expected 1 session remaining, got %d", c.ring.Len())
	}
}

func TestEpochUdpCrypto_Rekey_BadKey(t *testing.T) {
	c := makeUdpCrypto(t)
	_, err := c.Rekey([]byte("short"), []byte("short"))
	if err == nil {
		t.Fatal("expected error for invalid key length")
	}
}

func TestEpochUdpCrypto_Decrypt_EpochMismatch(t *testing.T) {
	c := makeUdpCrypto(t)
	r := &testEpochRing{
		resolveSession: newUdpSessionWithAEAD(7, badAEAD{}),
		resolveOK:      true,
	}
	c.ring = r

	buf := make([]byte, chacha20poly1305.NonceSize+chacha20poly1305.Overhead+1)
	binary.BigEndian.PutUint16(buf[NonceEpochOffset:NonceEpochOffset+2], 0)

	_, err := c.Decrypt(buf)
	if !errors.Is(err, ErrUnknownEpoch) {
		t.Fatalf("expected ErrUnknownEpoch on epoch mismatch, got %v", err)
	}
}

func TestEpochUdpCrypto_Rekey_BadRecvKey(t *testing.T) {
	c := makeUdpCrypto(t)
	_, err := c.Rekey(make([]byte, chacha20poly1305.KeySize), []byte("short"))
	if err == nil {
		t.Fatal("expected rekey recv key error")
	}
}

func TestEpochUdpCrypto_RemoveEpoch_NotFound(t *testing.T) {
	c := makeUdpCrypto(t)
	c.ring = &testEpochRing{
		lenVal:       2,
		removeResult: false,
	}
	c.SetSendEpoch(99)
	if c.RemoveEpoch(1) {
		t.Fatal("expected false when ring.Remove returns false")
	}
}

func TestEpochUdpCrypto_Zeroize(t *testing.T) {
	c := makeUdpCrypto(t)
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
