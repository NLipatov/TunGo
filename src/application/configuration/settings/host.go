package settings

// Host is the server address representation used in configuration files.
// A generated client configuration may contain both address families.
type Host struct {
	Domain string `json:"Domain,omitzero"`
	IPv4   string `json:"IPv4,omitzero"`
	IPv6   string `json:"IPv6,omitzero"`
}
