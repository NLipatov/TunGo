package nftables

import nft "github.com/google/nftables"

type conn interface {
	ListTables() ([]*nft.Table, error)
	ListChains() ([]*nft.Chain, error)
	AddTable(*nft.Table) *nft.Table
	AddChain(*nft.Chain) *nft.Chain
	GetRules(*nft.Table, *nft.Chain) ([]*nft.Rule, error)
	AddRule(*nft.Rule) *nft.Rule
	DelRule(*nft.Rule) error
	Flush() error
	CloseLasting() error
}
