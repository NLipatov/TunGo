package noise

import (
	"crypto/hmac"
	"crypto/rand"
	"fmt"
	"tungo/infrastructure/cryptography/mem"

	"golang.org/x/crypto/blake2s"
)

const (
	// ProtocolID is the protocol identifier for domain separation.
	ProtocolID = "TunGo"

	// ProtocolVersion is the current protocol version (IK pattern).
	// Version 0: Reserved (reject)
	// Version 1: Noise IK (current)
	// Version 2+: Reserved for future
	ProtocolVersion = 1

	// VersionSize is the size of the protocol version prefix.
	VersionSize = 1

	// MAC1Label is the label for MAC1 key derivation.
	MAC1Label = "mac1"

	// MAC2Label is the label for MAC2 key derivation.
	MAC2Label = "mac2"

	// CookieLabel is the label for cookie encryption key derivation.
	CookieLabel = "cookie"

	// MAC1Size is the size of MAC1 in bytes.
	MAC1Size = 16

	// MAC2Size is the size of MAC2 in bytes.
	MAC2Size = 16

	// EphemeralSize is the size of the ephemeral public key.
	EphemeralSize = 32

	// MinMsg1Size is the minimum size of Noise IK msg1.
	// ephemeral (32) + encrypted_static (32+16) = 80 bytes minimum
	MinMsg1Size = 80

	// MinTotalSize is the minimum total size including MACs (without version prefix).
	MinTotalSize = MinMsg1Size + MAC1Size + MAC2Size

	// MinTotalSizeWithVersion is the minimum total size including version prefix and MACs.
	// Wire format: [version (1)] [noise_msg1 (≥80)] [MAC1 (16)] [MAC2 (16)]
	MinTotalSizeWithVersion = VersionSize + MinTotalSize
)

// deriveMAC1Key derives the MAC1 key using BLAKE2s with full domain separation:
// key = BLAKE2s(label || protocol_id || version || server_pubkey)
func deriveMAC1Key(serverPubKey []byte) [32]byte {
	h, _ := blake2s.New256(nil)
	h.Write([]byte(MAC1Label))
	h.Write([]byte(ProtocolID))
	h.Write([]byte{byte(ProtocolVersion)})
	h.Write(serverPubKey)
	var key [32]byte
	copy(key[:], h.Sum(nil))
	return key
}

// ComputeMAC1 computes MAC1 over the Noise msg1 using keyed BLAKE2s-128.
// Uses BLAKE2s's built-in keyed MAC mode for efficiency and FIPS compatibility.
func ComputeMAC1(msg1, serverPubKey []byte) []byte {
	key := deriveMAC1Key(serverPubKey)
	defer mem.ZeroBytes(key[:])

	// BLAKE2s-128 with key
	h, _ := blake2s.New128(key[:])
	h.Write(msg1)
	return h.Sum(nil) // 16 bytes
}

// VerifyMAC1 verifies MAC1 on a message containing msg1 || MAC1 || MAC2.
// This is stateless and cheap - MUST be verified before any allocation or DH.
// Returns false if the message is too short or MAC1 is invalid.
func VerifyMAC1(msg1WithMAC, serverPubKey []byte) bool {
	if len(msg1WithMAC) < MinTotalSize {
		return false
	}

	msgLen := len(msg1WithMAC) - MAC1Size - MAC2Size
	msg1 := msg1WithMAC[:msgLen]
	mac1 := msg1WithMAC[msgLen : msgLen+MAC1Size]

	expected := ComputeMAC1(msg1, serverPubKey)
	return hmac.Equal(mac1, expected)
}

// deriveMAC2Key derives the MAC2 key from the cookie value:
// key = BLAKE2s(mac2_label || protocol_id || version || cookie_value)
func deriveMAC2Key(cookie []byte) [32]byte {
	h, _ := blake2s.New256(nil)
	h.Write([]byte(MAC2Label))
	h.Write([]byte(ProtocolID))
	h.Write([]byte{byte(ProtocolVersion)})
	h.Write(cookie)
	var key [32]byte
	copy(key[:], h.Sum(nil))
	return key
}

// ComputeMAC2 computes MAC2 over msg1 and MAC1 using the cookie.
// Uses BLAKE2s's built-in keyed MAC mode for efficiency and FIPS compatibility.
func ComputeMAC2(msg1, mac1, cookie []byte) []byte {
	key := deriveMAC2Key(cookie)
	defer mem.ZeroBytes(key[:])

	// BLAKE2s-128 with key
	h, _ := blake2s.New128(key[:])
	h.Write(msg1)
	h.Write(mac1)
	return h.Sum(nil) // 16 bytes
}

// VerifyMAC2 verifies MAC2 given the full message and the expected cookie.
func VerifyMAC2(msg1WithMAC, cookie []byte) bool {
	if len(msg1WithMAC) < MinTotalSize {
		return false
	}

	msgLen := len(msg1WithMAC) - MAC1Size - MAC2Size
	msg1 := msg1WithMAC[:msgLen]
	mac1 := msg1WithMAC[msgLen : msgLen+MAC1Size]
	mac2 := msg1WithMAC[msgLen+MAC1Size:]

	expected := ComputeMAC2(msg1, mac1, cookie)
	return hmac.Equal(mac2, expected)
}

// ExtractNoiseMsg extracts the Noise message from msg1WithMAC.
func ExtractNoiseMsg(msg1WithMAC []byte) []byte {
	if len(msg1WithMAC) < MinTotalSize {
		return nil
	}
	return msg1WithMAC[:len(msg1WithMAC)-MAC1Size-MAC2Size]
}

// ExtractClientEphemeral extracts the client's ephemeral public key from msg1.
// CRITICAL: This MUST only be called AFTER MAC1 verification succeeds.
// The ephemeral is the first 32 bytes of the Noise IK msg1 (always plaintext).
func ExtractClientEphemeral(msg1WithMAC []byte) []byte {
	if len(msg1WithMAC) < MinTotalSize {
		return nil
	}
	noiseMsg := ExtractNoiseMsg(msg1WithMAC)
	if len(noiseMsg) < EphemeralSize {
		return nil
	}
	ephemeral := make([]byte, EphemeralSize)
	copy(ephemeral, noiseMsg[:EphemeralSize])
	return ephemeral
}

// AppendMACs appends MAC1 and MAC2 to msg1.
// If cookie is nil or empty, MAC2 is filled with random bytes to avoid a DPI fingerprint.
func AppendMACs(msg1, serverPubKey, cookie []byte) ([]byte, error) {
	mac1 := ComputeMAC1(msg1, serverPubKey)

	result := make([]byte, len(msg1)+MAC1Size+MAC2Size)
	copy(result, msg1)
	copy(result[len(msg1):], mac1)

	if len(cookie) > 0 {
		mac2 := ComputeMAC2(msg1, mac1, cookie)
		copy(result[len(msg1)+MAC1Size:], mac2)
	} else {
		if _, err := rand.Read(result[len(msg1)+MAC1Size:]); err != nil {
			return nil, fmt.Errorf("crypto/rand failed: %w", err)
		}
	}

	return result, nil
}

// PrependVersion adds the protocol version byte to a message.
// Wire format: [version (1)] [msg...]
func PrependVersion(msg []byte) []byte {
	result := make([]byte, VersionSize+len(msg))
	result[0] = ProtocolVersion
	copy(result[VersionSize:], msg)
	return result
}

// CheckVersion verifies the protocol version byte and returns the message without it.
// Returns error if version is unsupported or message is too short.
// This MUST be called BEFORE MAC1 verification.
func CheckVersion(msgWithVersion []byte) ([]byte, error) {
	if len(msgWithVersion) < MinTotalSizeWithVersion {
		return nil, ErrMsgTooShort
	}

	version := msgWithVersion[0]
	switch version {
	case ProtocolVersion: // Version 1 = IK
		return msgWithVersion[VersionSize:], nil
	default:
		// Version 0 and 2+ are unknown/reserved
		return nil, ErrUnknownProtocol
	}
}
