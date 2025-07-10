package settings

import "crypto/rand"

// HelloMasking is used to mask hello packets between server and client from censorship's deep packet inspection (DPI).
type HelloMasking struct {
	// AEAD is a key, used for encryption and decryption operations (used in chacha20)
	AEAD []byte `json:"AEAD"`
	// HMAC is a key used for generating HMAC
	HMAC []byte `json:"HMAC"`
}

func NewHelloMasking() HelloMasking {
	return HelloMasking{
		AEAD: generate32byteKey(),
		HMAC: generate32byteKey(),
	}
}

func generate32byteKey() []byte {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		panic(err)
	}

	return key
}
