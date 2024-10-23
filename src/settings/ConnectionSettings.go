package settings

type ConnectionSettings struct {
	InterfaceName   string `json:"InterfaceName"`
	InterfaceIPCIDR string `json:"InterfaceIPCIDR"`
	ConnectionPort  string `json:"ConnectionPort"`
}
