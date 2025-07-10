package settings

// HelloMasking is used to mask hello packets between server and client from censorship's deep packet inspection (DPI).
type HelloMasking struct {
	// AEAD is a key, used for encryption and decryption operations (used in chacha20)
	AEAD []byte `json:"AEAD"`
	// HMAC is a key used for generating HMAC
	HMAC []byte `json:"HMAC"`
}
