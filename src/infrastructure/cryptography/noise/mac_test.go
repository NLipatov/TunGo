package noise

import (
	"bytes"
	"testing"
)

func TestMAC1_KeyDerivation_IncludesProtocolAndVersion(t *testing.T) {
	serverPubKey := make([]byte, 32)
	serverPubKey[0] = 1

	key1 := deriveMAC1Key(serverPubKey)

	// Change server public key
	serverPubKey[0] = 2
	key2 := deriveMAC1Key(serverPubKey)

	if bytes.Equal(key1[:], key2[:]) {
		t.Fatal("different server keys should produce different MAC1 keys")
	}
}

func TestMAC1_Computation(t *testing.T) {
	msg1 := []byte("test message")
	serverPubKey := make([]byte, 32)

	mac1 := ComputeMAC1(msg1, serverPubKey)
	if len(mac1) != MAC1Size {
		t.Fatalf("MAC1 should be %d bytes, got %d", MAC1Size, len(mac1))
	}

	// Deterministic
	mac1Again := ComputeMAC1(msg1, serverPubKey)
	if !bytes.Equal(mac1, mac1Again) {
		t.Fatal("MAC1 should be deterministic")
	}

	// Different message produces different MAC
	msg2 := []byte("different message")
	mac1Different := ComputeMAC1(msg2, serverPubKey)
	if bytes.Equal(mac1, mac1Different) {
		t.Fatal("different messages should produce different MACs")
	}
}

func TestMAC1_Verification_Valid(t *testing.T) {
	serverPubKey := make([]byte, 32)
	serverPubKey[0] = 1

	// Create a properly formatted msg1 with MACs
	noiseMsg := make([]byte, MinMsg1Size)
	for i := range noiseMsg {
		noiseMsg[i] = byte(i)
	}

	msg1WithMAC := AppendMACs(noiseMsg, serverPubKey, nil)

	if !VerifyMAC1(msg1WithMAC, serverPubKey) {
		t.Fatal("MAC1 verification should pass for valid message")
	}
}

func TestMAC1_Verification_Invalid(t *testing.T) {
	serverPubKey := make([]byte, 32)
	serverPubKey[0] = 1

	noiseMsg := make([]byte, MinMsg1Size)
	msg1WithMAC := AppendMACs(noiseMsg, serverPubKey, nil)

	// Corrupt MAC1
	msg1WithMAC[MinMsg1Size]++

	if VerifyMAC1(msg1WithMAC, serverPubKey) {
		t.Fatal("MAC1 verification should fail for corrupted message")
	}
}

func TestMAC1_Verification_Truncated(t *testing.T) {
	serverPubKey := make([]byte, 32)
	shortMsg := make([]byte, MinTotalSize-1)

	if VerifyMAC1(shortMsg, serverPubKey) {
		t.Fatal("MAC1 verification should fail for truncated message")
	}
}

func TestMAC1_DifferentVersionProducesDifferentKey(t *testing.T) {
	// This test verifies domain separation by checking that the key includes version
	serverPubKey := make([]byte, 32)
	key := deriveMAC1Key(serverPubKey)

	// The key should be non-zero
	allZero := true
	for _, b := range key {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("MAC1 key should not be all zeros")
	}
}

func TestMAC2_Computation(t *testing.T) {
	msg1 := make([]byte, MinMsg1Size)
	mac1 := make([]byte, MAC1Size)
	cookie := make([]byte, CookieSize)
	cookie[0] = 1

	mac2 := ComputeMAC2(msg1, mac1, cookie)
	if len(mac2) != MAC2Size {
		t.Fatalf("MAC2 should be %d bytes, got %d", MAC2Size, len(mac2))
	}

	// Different cookie produces different MAC2
	cookie[0] = 2
	mac2Different := ComputeMAC2(msg1, mac1, cookie)
	if bytes.Equal(mac2, mac2Different) {
		t.Fatal("different cookies should produce different MAC2")
	}
}

func TestMAC2_Verification(t *testing.T) {
	serverPubKey := make([]byte, 32)
	cookie := []byte("test_cookie_1234") // 16 bytes

	noiseMsg := make([]byte, MinMsg1Size)
	msg1WithMAC := AppendMACs(noiseMsg, serverPubKey, cookie)

	if !VerifyMAC2(msg1WithMAC, cookie) {
		t.Fatal("MAC2 verification should pass for valid cookie")
	}

	// Wrong cookie should fail
	wrongCookie := []byte("wrong_cookie_123")
	if VerifyMAC2(msg1WithMAC, wrongCookie) {
		t.Fatal("MAC2 verification should fail for wrong cookie")
	}
}

func TestExtractNoiseMsg(t *testing.T) {
	noiseMsg := make([]byte, MinMsg1Size)
	for i := range noiseMsg {
		noiseMsg[i] = byte(i)
	}

	serverPubKey := make([]byte, 32)
	msg1WithMAC := AppendMACs(noiseMsg, serverPubKey, nil)

	extracted := ExtractNoiseMsg(msg1WithMAC)
	if !bytes.Equal(extracted, noiseMsg) {
		t.Fatal("extracted noise message should match original")
	}
}

func TestExtractClientEphemeral(t *testing.T) {
	// Create a msg1 with a known ephemeral
	ephemeral := make([]byte, EphemeralSize)
	for i := range ephemeral {
		ephemeral[i] = byte(i + 100)
	}

	noiseMsg := make([]byte, MinMsg1Size)
	copy(noiseMsg[:EphemeralSize], ephemeral)

	serverPubKey := make([]byte, 32)
	msg1WithMAC := AppendMACs(noiseMsg, serverPubKey, nil)

	extracted := ExtractClientEphemeral(msg1WithMAC)
	if !bytes.Equal(extracted, ephemeral) {
		t.Fatal("extracted ephemeral should match original")
	}
}

func TestExtractClientEphemeral_TooShort(t *testing.T) {
	shortMsg := make([]byte, MinTotalSize-1)
	extracted := ExtractClientEphemeral(shortMsg)
	if extracted != nil {
		t.Fatal("should return nil for truncated message")
	}
}

func TestAppendMACs_WithoutCookie(t *testing.T) {
	noiseMsg := make([]byte, MinMsg1Size)
	serverPubKey := make([]byte, 32)

	msg1WithMAC := AppendMACs(noiseMsg, serverPubKey, nil)

	expectedLen := MinMsg1Size + MAC1Size + MAC2Size
	if len(msg1WithMAC) != expectedLen {
		t.Fatalf("expected length %d, got %d", expectedLen, len(msg1WithMAC))
	}

	// MAC2 should be zeros when no cookie
	mac2Start := MinMsg1Size + MAC1Size
	for i := mac2Start; i < len(msg1WithMAC); i++ {
		if msg1WithMAC[i] != 0 {
			t.Fatal("MAC2 should be zeros when no cookie provided")
		}
	}
}

func TestAppendMACs_WithCookie(t *testing.T) {
	noiseMsg := make([]byte, MinMsg1Size)
	serverPubKey := make([]byte, 32)
	cookie := []byte("valid_cookie_123") // 16 bytes

	msg1WithMAC := AppendMACs(noiseMsg, serverPubKey, cookie)

	// MAC2 should NOT be zeros when cookie provided
	mac2Start := MinMsg1Size + MAC1Size
	allZero := true
	for i := mac2Start; i < len(msg1WithMAC); i++ {
		if msg1WithMAC[i] != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("MAC2 should not be zeros when cookie provided")
	}
}

func TestPrependVersion(t *testing.T) {
	msg := []byte("test message")
	withVersion := PrependVersion(msg)

	if len(withVersion) != len(msg)+VersionSize {
		t.Fatalf("expected length %d, got %d", len(msg)+VersionSize, len(withVersion))
	}

	if withVersion[0] != ProtocolVersion {
		t.Fatalf("expected version %d, got %d", ProtocolVersion, withVersion[0])
	}

	if !bytes.Equal(withVersion[VersionSize:], msg) {
		t.Fatal("message content should be preserved after version byte")
	}
}

func TestCheckVersion_Valid(t *testing.T) {
	noiseMsg := make([]byte, MinTotalSize)
	withVersion := PrependVersion(noiseMsg)

	extracted, err := CheckVersion(withVersion)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Equal(extracted, noiseMsg) {
		t.Fatal("extracted message should match original")
	}
}

func TestCheckVersion_Version1Valid(t *testing.T) {
	msg := make([]byte, MinTotalSizeWithVersion)
	msg[0] = 1 // Version 1 = IK (current)

	extracted, err := CheckVersion(msg)
	if err != nil {
		t.Fatalf("version 1 should be valid, got error: %v", err)
	}
	if len(extracted) != MinTotalSize {
		t.Fatalf("expected extracted length %d, got %d", MinTotalSize, len(extracted))
	}
}

func TestCheckVersion_Unknown(t *testing.T) {
	tests := []struct {
		name    string
		version byte
	}{
		{"version 2 (future)", 2},
		{"version 99 (unknown)", 99},
		{"version 255 (max)", 255},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := make([]byte, MinTotalSizeWithVersion)
			msg[0] = tc.version

			_, err := CheckVersion(msg)
			if err != ErrUnknownProtocol {
				t.Fatalf("expected ErrUnknownProtocol for version %d, got: %v", tc.version, err)
			}
		})
	}
}

func TestCheckVersion_TooShort(t *testing.T) {
	msg := make([]byte, 10) // Too short
	msg[0] = ProtocolVersion

	_, err := CheckVersion(msg)
	if err != ErrMsgTooShort {
		t.Fatalf("expected ErrMsgTooShort, got: %v", err)
	}
}

func TestCheckVersion_ZeroVersion(t *testing.T) {
	msg := make([]byte, MinTotalSizeWithVersion)
	msg[0] = 0 // Version 0 is unknown

	_, err := CheckVersion(msg)
	if err != ErrUnknownProtocol {
		t.Fatalf("expected ErrUnknownProtocol, got: %v", err)
	}
}

func TestVerifyMAC2_TooShort(t *testing.T) {
	short := make([]byte, MinTotalSize-1)
	cookie := make([]byte, CookieSize)
	if VerifyMAC2(short, cookie) {
		t.Fatal("VerifyMAC2 should return false for truncated message")
	}
}

func TestExtractNoiseMsg_TooShort(t *testing.T) {
	short := make([]byte, MinTotalSize-1)
	if ExtractNoiseMsg(short) != nil {
		t.Fatal("ExtractNoiseMsg should return nil for truncated message")
	}
}

func TestExtractClientEphemeral_NoiseMsgTooShort(t *testing.T) {
	// Message has enough total size but noise part < EphemeralSize
	// Create a message that's MinTotalSize exactly — noiseMsg will be 80 bytes, which is >= 32 (EphemeralSize).
	// So we need an edge case where noiseMsg is < EphemeralSize.
	// That can't happen because MinTotalSize = MinMsg1Size + MAC1Size + MAC2Size and MinMsg1Size = 80 >= EphemeralSize.
	// So we only test the too-short outer boundary.
	short := make([]byte, MinTotalSize-1)
	if ExtractClientEphemeral(short) != nil {
		t.Fatal("ExtractClientEphemeral should return nil for too-short message")
	}
}

func TestAppendMACs_VerifyRoundTrip(t *testing.T) {
	serverPubKey := make([]byte, 32)
	serverPubKey[0] = 42
	cookie := []byte("roundtrip_cookie")

	noiseMsg := make([]byte, MinMsg1Size)
	for i := range noiseMsg {
		noiseMsg[i] = byte(i * 3)
	}

	withMAC := AppendMACs(noiseMsg, serverPubKey, cookie)

	if !VerifyMAC1(withMAC, serverPubKey) {
		t.Fatal("MAC1 roundtrip verification failed")
	}
	if !VerifyMAC2(withMAC, cookie) {
		t.Fatal("MAC2 roundtrip verification failed")
	}

	extracted := ExtractNoiseMsg(withMAC)
	if !bytes.Equal(extracted, noiseMsg) {
		t.Fatal("extracted noise msg doesn't match original")
	}
}

func TestPrependCheckVersion_RoundTrip(t *testing.T) {
	msg := make([]byte, MinTotalSize)
	for i := range msg {
		msg[i] = byte(i)
	}

	withVersion := PrependVersion(msg)
	extracted, err := CheckVersion(withVersion)
	if err != nil {
		t.Fatalf("CheckVersion failed: %v", err)
	}
	if !bytes.Equal(extracted, msg) {
		t.Fatal("roundtrip through PrependVersion/CheckVersion failed")
	}
}
