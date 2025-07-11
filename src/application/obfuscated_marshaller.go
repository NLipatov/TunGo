package application

type Obfuscator interface {
	Obfuscate([]byte) ([]byte, error)
	Deobfuscate([]byte) ([]byte, error)
}
