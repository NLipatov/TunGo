package service

type PacketType uint8

const (
	Unknown PacketType = iota
	SessionReset
	RekeyInit
	RekeyAck
)

func PacketTypeIs(raw byte, t PacketType) bool {
	return PacketType(raw) == t
}
