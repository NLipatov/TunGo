package tun_udp_chacha20

type clientPacket struct {
	client  *clientData
	payload []byte
}
