// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// tableIdentity is built from every selected profile before collection-time
// profile omission can change the active table set.
type tableIdentity struct {
	// Ambiguous names with multiple symbol-bearing owner OIDs are absent.
	canonicalByName map[string]string
}

var emptyTableIdentity = &tableIdentity{}

func buildTableIdentity(profiles []*ddsnmp.Profile) *tableIdentity {
	var owners map[string]map[string]bool
	addOwner := func(cfg ddprofiledefinition.MetricsConfig) {
		if cfg.IsScalar() || cfg.Table.OID == "" || cfg.Table.Name == "" || len(cfg.Symbols) == 0 {
			return
		}
		if owners == nil {
			owners = make(map[string]map[string]bool)
		}
		if owners[cfg.Table.Name] == nil {
			owners[cfg.Table.Name] = make(map[string]bool)
		}
		owners[cfg.Table.Name][cfg.Table.OID] = true
	}

	for _, prof := range profiles {
		if prof == nil || prof.Definition == nil {
			continue
		}
		for _, cfg := range prof.Definition.Metrics {
			addOwner(cfg)
		}
		for _, topology := range prof.Definition.Topology {
			addOwner(topology.MetricsConfig)
		}
	}

	if len(owners) == 0 {
		return emptyTableIdentity
	}

	canonical := make(map[string]string)
	for name, tableOIDs := range owners {
		if len(tableOIDs) != 1 {
			continue
		}
		for tableOID := range tableOIDs {
			canonical[name] = tableOID
		}
	}
	return &tableIdentity{canonicalByName: canonical}
}
