package serviceframe

const (
	HeaderSize = 7 // 2(Magic)+1(Version)+1(Kind)+1(Flags)+2(Payload Len Size)
	MaxBody    = 1500
)
