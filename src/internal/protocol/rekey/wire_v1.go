package rekey

// Rekey V1 carries one 32-byte X25519 public key after the service header.
const (
	serviceHeaderLen = 3
	v1PublicKeyLen   = 32
	v1PacketLen      = serviceHeaderLen + v1PublicKeyLen
)
