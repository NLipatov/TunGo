package ioctl

const (
	ifNamSiz  = 16         // Max if name size, bytes
	tunSetIff = 0x400454ca // Code to create TUN/TAP if via ioctl
	iffTun    = 0x0001     // Enabling TUN flag
	IffNoPi   = 0x1000     // Disabling PI (Packet Information)
)
