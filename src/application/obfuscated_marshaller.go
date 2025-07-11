package application

import "encoding"

type ObfuscatableData interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

type Obfuscator[T ObfuscatableData] interface {
	Obfuscate(T) ([]byte, error)
	Deobfuscate([]byte) (T, error)
}
