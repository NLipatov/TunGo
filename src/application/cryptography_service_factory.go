package application

type CryptographyServiceFactory interface {
	FromHandshake(
		handshake Handshake,
		isServer bool,
	) (CryptographyService, error)
}
