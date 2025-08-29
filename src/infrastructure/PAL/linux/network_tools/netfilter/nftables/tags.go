package nftables

type Tags interface {
	tagMasq4(dev string) []byte
	tagMasq6(dev string) []byte
	tagV4Fwd(iif, oif string) []byte
	tagV4FwdRet(iif, oif string) []byte
	tagV6Fwd(iif, oif string) []byte
	tagV6FwdRet(iif, oif string) []byte
	tagHookJump(child string) []byte
}

type DefaultTags struct {
}

func NewDefaultTags() *DefaultTags {
	return &DefaultTags{}
}

func (t DefaultTags) tagMasq4(dev string) []byte { return []byte("tungo:nat4 oif=" + dev) }
func (t DefaultTags) tagMasq6(dev string) []byte { return []byte("tungo:nat6 oif=" + dev) }

func (t DefaultTags) tagV4Fwd(iif, oif string) []byte {
	return []byte("tungo:v4 fwd " + iif + "->" + oif)
}
func (t DefaultTags) tagV4FwdRet(iif, oif string) []byte {
	return []byte("tungo:v4 fwdret " + iif + "->" + oif)
}
func (t DefaultTags) tagV6Fwd(iif, oif string) []byte {
	return []byte("tungo:v6 fwd " + iif + "->" + oif)
}
func (t DefaultTags) tagV6FwdRet(iif, oif string) []byte {
	return []byte("tungo:v6 fwdret " + iif + "->" + oif)
}

func (t DefaultTags) tagHookJump(child string) []byte { return []byte("tungo:hook FORWARD->" + child) }
