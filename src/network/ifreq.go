package network

type IfReq struct {
	Name  [IFNAMSIZ]byte
	Flags uint16
	_     [24]byte
}
