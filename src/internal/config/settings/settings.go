package settings

type Settings struct {
	Network
	MTU           int           `json:"MTU"`
	Protocol      Protocol      `json:"Protocol"`
	Encryption    Encryption    `json:"Encryption"`
	DialTimeoutMs DialTimeoutMs `json:"DialTimeoutMs"`
}
