package ip

type IfReq struct {
	Name  [ifNamSiz]byte
	Flags uint16
	_     [24]byte
}
