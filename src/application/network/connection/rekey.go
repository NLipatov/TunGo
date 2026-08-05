package connection

// RekeyController is the application-level capability exposed by an
// established connection. Consumers should depend on narrower local
// interfaces when they need only part of this API.
type RekeyController interface {
	ReadyForRekey() bool
	SendEpoch() uint16
	StartRekey(c2s, s2c []byte) (uint16, error)
	ActivateSendEpoch(epoch uint16)
	ObservePeerEpoch(epoch uint16)
	// CurrentKeys returns caller-owned snapshots of the directional keys.
	CurrentKeys() (clientToServer, serverToClient []byte)
}
