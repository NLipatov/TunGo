package settings

type ConnectionSettings struct {
	InterfaceName    string `json:"InterfaceName"`
	InterfaceIPCIDR  string `json:"InterfaceIPCIDR"`
	InterfaceAddress string `json:"InterfaceAddress"`
	ConnectionPort   string `json:"ConnectionPort"`
}
