package application

type CryptographyServiceBuilder interface {
	FromHandshake(handshake Handshake, isServer bool) (CryptographyService, error)
}
