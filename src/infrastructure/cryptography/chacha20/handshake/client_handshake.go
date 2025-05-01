package handshake

import (
	"net"
	"tungo/settings"

	"golang.org/x/crypto/ed25519"
)

func prepareClientCredentials() (
	edPub ed25519.PublicKey, edPriv ed25519.PrivateKey,
	sessPub []byte, sessPriv [32]byte, salt []byte, err error,
) {
	cc := NewDefaultClientCrypto()
	edPub, edPriv, err = cc.GenerateEd25519Keys()
	if err != nil {
		return
	}
	sessPub, sessPriv, err = cc.NewX25519SessionKeyPair()
	if err != nil {
		return
	}
	salt = cc.GenerateSalt()
	return
}

func sendClientHello(conn net.Conn, ip string, edPub ed25519.PublicKey, sessPub, salt []byte) error {
	io := NewDefaultClientIO(conn, settings.ConnectionSettings{InterfaceAddress: ip}, edPub, sessPub, salt)
	return io.SendClientHello()
}

func receiveAndVerifyServerHello(conn net.Conn, edPub ed25519.PublicKey, salt []byte) (ServerHello, error) {
	io := NewDefaultClientIO(conn, settings.ConnectionSettings{}, edPub, nil, salt)
	sh, err := io.ReceiveServerHello()
	if err != nil {
		return ServerHello{}, err
	}
	// verify signature…
	return sh, nil
}

func signAndSendClientSignature(
	conn net.Conn, edPriv ed25519.PrivateKey,
	sessPub, salt, serverNonce []byte,
) error {
	data := append(append(sessPub, salt...), serverNonce...)
	sig := ed25519.Sign(edPriv, data)
	io := NewDefaultClientIO(conn, settings.ConnectionSettings{}, nil, nil, nil)
	return io.SendClientSignature(sig)
}

func (h *HandshakeImpl) finishKeysAndID(
	sessPriv [32]byte, salt []byte, sh ServerHello,
) error {
	crypto := NewDefaultCrypto()
	cc := NewDefaultClientCrypto()
	shared, err := cc.GenerateSharedSecret(sessPriv[:], sh.CurvePublicKey)
	if err != nil {
		return err
	}
	s2c, c2s, err := crypto.deriveTwoKeys(shared, salt, sh.Nonce)
	if err != nil {
		return err
	}
	id, err := NewDefaultSessionIdDeriver(shared, salt).Derive()
	if err != nil {
		return err
	}
	h.serverKey = s2c
	h.clientKey = c2s
	h.id = id
	return nil
}
