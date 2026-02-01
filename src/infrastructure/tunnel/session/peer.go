package session

import "tungo/application/network/connection"

// Peer is a session paired with its egress path — the unit stored in Repository.
type Peer struct {
	connection.Session
	egress connection.Egress
}

func NewPeer(session connection.Session, egress connection.Egress) *Peer {
	return &Peer{Session: session, egress: egress}
}

func (p *Peer) Egress() connection.Egress {
	return p.egress
}
