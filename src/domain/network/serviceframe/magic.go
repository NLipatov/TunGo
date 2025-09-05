package serviceframe

// Magic bytes for service frame identification.
const (
	MagicByte1 = 'S'
	MagicByte2 = 'F'
)

// MagicSF is unique marker indicating service frame(SF).
var MagicSF = [2]byte{MagicByte1, MagicByte2}
