package settings

type Settings struct {
	InterfaceName    string `json:"InterfaceName"`
	InterfaceIPCIDR  string `json:"InterfaceIPCIDR"`
	InterfaceAddress string `json:"InterfaceAddress"`
	ConnectionIP     string `json:"ConnectionIP"`
	Port             string `json:"Port"`
	MTU              int    `json:"MTU"`
	Protocol         Protocol
	Encryption       Encryption
	DialTimeoutMs    int             `json:"DialTimeoutMs"`
	SessionLifetime  SessionLifetime `json:"SessionLifetime"`
}
