package serviceframe

// Flags is a mask for additional frame features.
// Flags are reserved for future use.
type Flags uint8

const (
	FlagNone Flags = 0
)

func (f Flags) String() string {
	switch f {
	case FlagNone:
		return "None"
	default:
		return "Unknown"
	}
}
