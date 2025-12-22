//go:build darwin

package scutil

import (
	"fmt"
	"strings"
)

// dnsScriptBuilder builds scutil DSL scripts for DNS configuration.
// It is a pure builder with no side effects.
type dnsScriptBuilder struct {
	ifName      string
	nameservers []string
	domains     []string
}

// newDNSScriptBuilder creates a new DNS script builder.
func newDNSScriptBuilder(
	ifName string,
	nameservers []string,
	domains []string,
) *dnsScriptBuilder {
	if len(domains) == 0 {
		domains = []string{"~."}
	}

	return &dnsScriptBuilder{
		ifName:      ifName,
		nameservers: nameservers,
		domains:     domains,
	}
}

// BuildAdd builds a script that installs a scoped DNS resolver.
func (b *dnsScriptBuilder) BuildAdd() string {
	var sb strings.Builder

	sb.WriteString("d.init\n")

	sb.WriteString("d.add ServerAddresses * ")
	sb.WriteString(strings.Join(b.nameservers, " "))
	sb.WriteString("\n")

	sb.WriteString("d.add SupplementalMatchDomains * ")
	sb.WriteString(strings.Join(b.domains, " "))
	sb.WriteString("\n")

	sb.WriteString("d.add SupplementalMatchDomainsNoSearch 1\n")

	sb.WriteString(fmt.Sprintf(
		"set State:/Network/Service/%s/DNS\n",
		b.ifName,
	))

	return sb.String()
}

// BuildRemove builds a script that removes the scoped DNS resolver.
func (b *dnsScriptBuilder) BuildRemove() string {
	return fmt.Sprintf(
		"remove State:/Network/Service/%s/DNS\n",
		b.ifName,
	)
}
