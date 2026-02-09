package noise

import (
	"bytes"
	"net/netip"
	"testing"
	"time"
)

func TestCookie_ValueComputation(t *testing.T) {
	var secret [32]byte
	secret[0] = 1
	cm := NewCookieManagerWithSecret(secret)

	clientIP := netip.MustParseAddr("192.168.1.100")
	cookie := cm.ComputeCookieValue(clientIP)

	if len(cookie) != CookieSize {
		t.Fatalf("cookie should be %d bytes, got %d", CookieSize, len(cookie))
	}

	// Deterministic within same time bucket
	cookie2 := cm.ComputeCookieValue(clientIP)
	if !bytes.Equal(cookie, cookie2) {
		t.Fatal("cookie should be deterministic within same time bucket")
	}

	// Different IP produces different cookie
	differentIP := netip.MustParseAddr("192.168.1.101")
	cookieDifferent := cm.ComputeCookieValue(differentIP)
	if bytes.Equal(cookie, cookieDifferent) {
		t.Fatal("different IPs should produce different cookies")
	}
}

func TestCookie_EncryptionDecryption(t *testing.T) {
	cm, err := NewCookieManager()
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	clientIP := netip.MustParseAddr("10.0.0.5")
	clientEphemeral := make([]byte, EphemeralSize)
	for i := range clientEphemeral {
		clientEphemeral[i] = byte(i)
	}
	serverPubKey := make([]byte, 32)
	serverPubKey[0] = 1

	reply, err := cm.CreateCookieReply(clientIP, clientEphemeral, serverPubKey)
	if err != nil {
		t.Fatalf("failed to create cookie reply: %v", err)
	}

	if len(reply) != CookieReplySize {
		t.Fatalf("cookie reply should be %d bytes, got %d", CookieReplySize, len(reply))
	}

	// Decrypt
	decrypted, err := DecryptCookieReply(reply, clientEphemeral, serverPubKey)
	if err != nil {
		t.Fatalf("failed to decrypt cookie reply: %v", err)
	}

	// Should match computed cookie value
	expected := cm.ComputeCookieValue(clientIP)
	if !bytes.Equal(decrypted, expected) {
		t.Fatal("decrypted cookie should match computed value")
	}
}

func TestCookie_BoundToClientEphemeral(t *testing.T) {
	cm, err := NewCookieManager()
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	clientIP := netip.MustParseAddr("10.0.0.5")
	serverPubKey := make([]byte, 32)

	ephemeral1 := make([]byte, EphemeralSize)
	ephemeral1[0] = 1

	ephemeral2 := make([]byte, EphemeralSize)
	ephemeral2[0] = 2

	reply, err := cm.CreateCookieReply(clientIP, ephemeral1, serverPubKey)
	if err != nil {
		t.Fatalf("failed to create cookie reply: %v", err)
	}

	// Decrypting with wrong ephemeral should fail
	_, err = DecryptCookieReply(reply, ephemeral2, serverPubKey)
	if err == nil {
		t.Fatal("decryption should fail with different ephemeral")
	}
}

func TestCookie_Expiry_CurrentBucket(t *testing.T) {
	var secret [32]byte
	secret[0] = 1
	cm := NewCookieManagerWithSecret(secret)

	clientIP := netip.MustParseAddr("10.0.0.5")
	cookie := cm.ComputeCookieValue(clientIP)

	if !cm.ValidateCookie(clientIP, cookie) {
		t.Fatal("cookie should be valid in current bucket")
	}
}

func TestCookie_Expiry_PreviousBucket(t *testing.T) {
	var secret [32]byte
	secret[0] = 1
	cm := NewCookieManagerWithSecret(secret)

	clientIP := netip.MustParseAddr("10.0.0.5")

	// Generate cookie at a fixed time
	fixedTime := time.Unix(1000*CookieBucketSeconds, 0)
	cm.now = func() time.Time { return fixedTime }
	cookie := cm.ComputeCookieValue(clientIP)

	// Move to next bucket
	cm.now = func() time.Time { return fixedTime.Add(time.Duration(CookieBucketSeconds) * time.Second) }

	if !cm.ValidateCookie(clientIP, cookie) {
		t.Fatal("cookie should be valid in previous bucket (transition period)")
	}
}

func TestCookie_Expiry_TooOld(t *testing.T) {
	var secret [32]byte
	secret[0] = 1
	cm := NewCookieManagerWithSecret(secret)

	clientIP := netip.MustParseAddr("10.0.0.5")

	// Generate cookie at a fixed time
	fixedTime := time.Unix(1000*CookieBucketSeconds, 0)
	cm.now = func() time.Time { return fixedTime }
	cookie := cm.ComputeCookieValue(clientIP)

	// Move two buckets ahead (too old)
	cm.now = func() time.Time { return fixedTime.Add(time.Duration(2*CookieBucketSeconds) * time.Second) }

	if cm.ValidateCookie(clientIP, cookie) {
		t.Fatal("cookie should be invalid when too old")
	}
}

func TestCookie_SecretRotation(t *testing.T) {
	cm, err := NewCookieManager()
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	clientIP := netip.MustParseAddr("10.0.0.5")
	cookie1 := cm.ComputeCookieValue(clientIP)

	// Rotate secret
	if err := cm.RotateSecret(); err != nil {
		t.Fatalf("failed to rotate secret: %v", err)
	}

	cookie2 := cm.ComputeCookieValue(clientIP)

	if bytes.Equal(cookie1, cookie2) {
		t.Fatal("cookies should differ after secret rotation")
	}

	// Old cookie should not validate with new secret
	if cm.ValidateCookie(clientIP, cookie1) {
		t.Fatal("old cookie should not validate after secret rotation")
	}
}

func TestCookie_ReplayWithDifferentEphemeral(t *testing.T) {
	cm, err := NewCookieManager()
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	clientIP := netip.MustParseAddr("10.0.0.5")
	serverPubKey := make([]byte, 32)

	// Original ephemeral
	ephemeral1 := make([]byte, EphemeralSize)
	ephemeral1[0] = 1

	// Create cookie reply bound to original ephemeral
	reply, err := cm.CreateCookieReply(clientIP, ephemeral1, serverPubKey)
	if err != nil {
		t.Fatalf("failed to create cookie reply: %v", err)
	}

	// Attacker tries to use cookie with their own ephemeral
	attackerEphemeral := make([]byte, EphemeralSize)
	attackerEphemeral[0] = 99

	_, err = DecryptCookieReply(reply, attackerEphemeral, serverPubKey)
	if err == nil {
		t.Fatal("decryption should fail when ephemeral doesn't match")
	}
}

func TestIsCookieReply(t *testing.T) {
	// Correct size
	reply := make([]byte, CookieReplySize)
	if !IsCookieReply(reply) {
		t.Fatal("should identify cookie reply by size")
	}

	// Wrong size
	wrongSize := make([]byte, CookieReplySize+1)
	if IsCookieReply(wrongSize) {
		t.Fatal("should reject wrong size as cookie reply")
	}

	// Too short
	tooShort := make([]byte, CookieReplySize-1)
	if IsCookieReply(tooShort) {
		t.Fatal("should reject too short as cookie reply")
	}
}

func TestDecryptCookieReply_InvalidFormat(t *testing.T) {
	clientEphemeral := make([]byte, EphemeralSize)
	serverPubKey := make([]byte, 32)

	// Too short
	tooShort := make([]byte, CookieNonceSize)
	_, err := DecryptCookieReply(tooShort, clientEphemeral, serverPubKey)
	if err == nil {
		t.Fatal("should fail for too short reply")
	}

	// Corrupted ciphertext
	corrupted := make([]byte, CookieReplySize)
	_, err = DecryptCookieReply(corrupted, clientEphemeral, serverPubKey)
	if err == nil {
		t.Fatal("should fail for corrupted ciphertext")
	}
}

func TestDecryptCookieReply_ExactMinSize(t *testing.T) {
	clientEphemeral := make([]byte, EphemeralSize)
	serverPubKey := make([]byte, 32)

	// Exactly nonce size + overhead: too short for any plaintext content
	minInvalid := make([]byte, CookieNonceSize+16) // nonce + tag but no ciphertext content
	_, err := DecryptCookieReply(minInvalid, clientEphemeral, serverPubKey)
	if err == nil {
		t.Fatal("should fail for minimum-size invalid reply")
	}
}

func TestCookie_ValidateCookie_WrongIP_Fails(t *testing.T) {
	var secret [32]byte
	secret[0] = 1
	cm := NewCookieManagerWithSecret(secret)

	clientIP := netip.MustParseAddr("10.0.0.1")
	wrongIP := netip.MustParseAddr("10.0.0.2")
	cookie := cm.ComputeCookieValue(clientIP)

	if cm.ValidateCookie(wrongIP, cookie) {
		t.Fatal("cookie should not validate for different IP")
	}
}

func TestCookie_ValidateCookie_GarbageCookie_Fails(t *testing.T) {
	var secret [32]byte
	secret[0] = 1
	cm := NewCookieManagerWithSecret(secret)

	clientIP := netip.MustParseAddr("10.0.0.1")
	garbage := make([]byte, CookieSize)
	garbage[0] = 0xff

	if cm.ValidateCookie(clientIP, garbage) {
		t.Fatal("garbage cookie should not validate")
	}
}

func TestCookie_IsCookieReply_Empty(t *testing.T) {
	if IsCookieReply(nil) {
		t.Fatal("nil should not be cookie reply")
	}
	if IsCookieReply([]byte{}) {
		t.Fatal("empty slice should not be cookie reply")
	}
}

func TestCookie_IPv6Client(t *testing.T) {
	cm, err := NewCookieManager()
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	clientIP := netip.MustParseAddr("2001:db8::1")
	cookie := cm.ComputeCookieValue(clientIP)
	if len(cookie) != CookieSize {
		t.Fatalf("expected %d bytes, got %d", CookieSize, len(cookie))
	}
	if !cm.ValidateCookie(clientIP, cookie) {
		t.Fatal("cookie should validate for IPv6 client")
	}
}
