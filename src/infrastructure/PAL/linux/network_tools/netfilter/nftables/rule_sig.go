package nftables

import "github.com/google/nftables/expr"

type RuleSig struct {
	kind        string // "masq" | "fwd" | "jump"
	iif, oif    string
	established bool
	jumpChain   string
}

type RuleSigHandler interface {
	sigMasq(oif string) RuleSig
	sigFwd(iif, oif string, established bool) RuleSig
	sigJump(chain string) RuleSig
	sigEqual(a, b RuleSig) bool
	sigFromExprs(exprs []expr.Any) (RuleSig, bool)
}

type DefaultRuleSigHandler struct {
}

func NewDefaultRuleSigHandler() *DefaultRuleSigHandler {
	return &DefaultRuleSigHandler{}
}

func (r *DefaultRuleSigHandler) sigMasq(oif string) RuleSig {
	return RuleSig{kind: "masq", oif: oif}
}
func (r *DefaultRuleSigHandler) sigFwd(iif, oif string, established bool) RuleSig {
	return RuleSig{kind: "fwd", iif: iif, oif: oif, established: established}
}
func (r *DefaultRuleSigHandler) sigJump(chain string) RuleSig {
	return RuleSig{kind: "jump", jumpChain: chain}
}

func (r *DefaultRuleSigHandler) sigEqual(a, b RuleSig) bool {
	return a.kind == b.kind &&
		a.iif == b.iif &&
		a.oif == b.oif &&
		a.established == b.established &&
		a.jumpChain == b.jumpChain
}

func (r *DefaultRuleSigHandler) sigFromExprs(exprs []expr.Any) (RuleSig, bool) {
	var iif, oif string
	var lastMeta string
	var sawCT, sawBitMask, sawCmpNonZero, sawMasq, accept bool
	var jumpTo string

	for _, e := range exprs {
		switch x := e.(type) {
		case *expr.Meta:
			if x.Register == 1 {
				switch x.Key {
				case expr.MetaKeyIIFNAME:
					lastMeta = "iif"
				case expr.MetaKeyOIFNAME:
					lastMeta = "oif"
				default:
					lastMeta = ""
				}
			}
		case *expr.Cmp:
			if x.Register == 1 && x.Op == expr.CmpOpEq &&
				len(x.Data) > 0 && x.Data[len(x.Data)-1] == 0x00 {
				name := string(x.Data[:len(x.Data)-1])
				if lastMeta == "iif" {
					iif = name
				} else if lastMeta == "oif" {
					oif = name
				}
				lastMeta = ""
			}
			if x.Register == 1 && x.Op == expr.CmpOpNeq &&
				len(x.Data) == 4 &&
				x.Data[0] == 0 && x.Data[1] == 0 && x.Data[2] == 0 && x.Data[3] == 0 {
				sawCmpNonZero = true
			}
		case *expr.Ct:
			if x.Register == 1 && x.Key == expr.CtKeySTATE {
				sawCT = true
			}
		case *expr.Bitwise:
			if x.DestRegister == 1 && x.Len == 4 && len(x.Mask) == 4 {
				sawBitMask = true
			}
		case *expr.Masq:
			sawMasq = true
		case *expr.Verdict:
			if x.Kind == expr.VerdictAccept {
				accept = true
			}
			if x.Kind == expr.VerdictJump {
				jumpTo = x.Chain
			}
		}
	}

	if jumpTo != "" {
		return RuleSig{kind: "jump", jumpChain: jumpTo}, true
	}
	if iif == "" && oif != "" && sawMasq {
		return RuleSig{kind: "masq", oif: oif}, true
	}
	if iif != "" && oif != "" && accept {
		est := sawCT && sawBitMask && sawCmpNonZero
		return RuleSig{kind: "fwd", iif: iif, oif: oif, established: est}, true
	}
	return RuleSig{}, false
}
