package application

type ObfuscatedMarshaler interface {
	MarshalObfuscatedBinary() ([]byte, error)
	UnmarshalObfuscatedBinary([]byte) error
}
