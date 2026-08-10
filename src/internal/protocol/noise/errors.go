package noise

import "errors"

// Handshake errors - internal use only.
// External responses MUST be uniform to prevent information leakage.
var (
	// ErrInvalidMAC1 indicates MAC1 verification failed.
	ErrInvalidMAC1 = errors.New("MAC1 verification failed")

	// ErrInvalidMAC2 indicates MAC2 verification failed.
	ErrInvalidMAC2 = errors.New("MAC2 verification failed")

	// ErrCookieRequired indicates the server is under load and requires a cookie.
	ErrCookieRequired = errors.New("cookie required")

	// ErrInvalidCookieReply indicates the cookie reply format is invalid.
	ErrInvalidCookieReply = errors.New("invalid cookie reply")

	// ErrUnknownPeer indicates the client's public key is not in AllowedPeers.
	ErrUnknownPeer = errors.New("unknown peer")

	// ErrPeerDisabled indicates the client is disabled in AllowedPeers.
	ErrPeerDisabled = errors.New("peer disabled")

	// ErrUnknownProtocol indicates an unknown protocol version.
	ErrUnknownProtocol = errors.New("unknown protocol version")

	// ErrMsgTooShort indicates the message is too short.
	ErrMsgTooShort = errors.New("message too short")

	// ErrHandshakeFailed is the uniform external error for any handshake failure.
	// This error MUST be returned to external clients to prevent information leakage.
	ErrHandshakeFailed = errors.New("handshake failed")

	// ErrMissingClientKey indicates client keys are not configured.
	ErrMissingClientKey = errors.New("client keys not configured")

	// ErrMissingServerKey indicates server keys are not configured.
	ErrMissingServerKey = errors.New("server keys not configured")

	// ErrMissingAllowedPeers indicates AllowedPeers is not configured.
	ErrMissingAllowedPeers = errors.New("allowed peers not configured")
)
