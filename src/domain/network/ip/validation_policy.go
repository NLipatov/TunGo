package ip

type ValidationPolicy struct {
	AllowV4           bool
	AllowV6           bool
	RequirePrivate    bool // RFC1918 for v4, ULA fc00::/7 for v6
	ForbidLoopback    bool
	ForbidMulticast   bool
	ForbidUnspecified bool
	ForbidLinkLocal   bool // v4: 169.254/16, v6: fe80::/10
	ForbidBroadcastV4 bool // 255.255.255.255
}
