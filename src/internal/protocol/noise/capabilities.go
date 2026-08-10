package noise

// Capability identifies an optional feature advertised in the authenticated
// Noise handshake payload. A capability set is encoded as one byte per entry.
type Capability byte

const (
	CapabilityUnknown Capability = iota
	CapabilityRekeyV2
)
