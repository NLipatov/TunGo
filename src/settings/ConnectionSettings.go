package settings

type ConnectionSettings struct {
	InterfaceName    string `json:"InterfaceName"`
	InterfaceIPCIDR  string `json:"InterfaceIPCIDR"`
	InterfaceAddress string `json:"InterfaceAddress"`
	ConnectionIP     string `json:"ConnectionIP"`
	ConnectionPort   string `json:"ConnectionPort"`
}
