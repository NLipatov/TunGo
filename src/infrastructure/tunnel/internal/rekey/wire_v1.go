package rekey

// Rekey V1 carries one 32-byte X25519 public key after the service header.
const (
	v1ServiceHeaderLen = 3
	v1PublicKeyLen     = 32
	v1PacketLen        = v1ServiceHeaderLen + v1PublicKeyLen
)
