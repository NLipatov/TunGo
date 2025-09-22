package service

type PacketHandler interface {
	TryParseType(pkt []byte) (PacketType, bool)
	EncodeLegacy(t PacketType, buffer []byte) ([]byte, error)
	EncodeV1(t PacketType, buffer []byte) ([]byte, error)
}
