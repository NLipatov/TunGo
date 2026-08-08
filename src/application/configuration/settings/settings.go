package settings

type Settings struct {
	Addressing
	MTU           int           `json:"MTU"`
	Protocol      Protocol      `json:"Protocol"`
	Encryption    Encryption    `json:"Encryption"`
	DialTimeoutMs DialTimeoutMs `json:"DialTimeoutMs"`
}
