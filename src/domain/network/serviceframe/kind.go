package serviceframe

type Kind uint8

// Session Control Group
const (
	// KindSessionReset is used to reestablish cryptography session
	KindSessionReset Kind = 1
)

// MTU Control Group
const (
	// KindMTUProbe is used to check whether current MTU value is ok or not
	KindMTUProbe Kind = 10
	// KindMTUAck is used to respond to KindMTUProbe
	KindMTUAck Kind = 11
)

func (k Kind) IsValid() bool {
	switch k {
	case KindSessionReset, KindMTUProbe, KindMTUAck:
		return true
	default:
		return false
	}
}

func (k Kind) String() string {
	switch k {
	case KindSessionReset:
		return "SessionRST"
	case KindMTUProbe:
		return "MTUProbe"
	case KindMTUAck:
		return "MTUAck"
	default:
		return "Unknown"
	}
}
