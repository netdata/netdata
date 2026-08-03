// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/enrichment"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/enrichment/netdataadapter"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
)

const testEngineIDHex = "80001f888077dfe44faa700258"

func newTestSNMPTrapsCollector() *Collector {
	return New(ddsnmp.NewDeviceStore(), snmptopology.NewTrapEnrichmentHandle(), newTestReverseDNSResolver())
}

func newTestSNMPTrapsCollectorWithCatalog(manager *catalog.Manager) *Collector {
	c := newTestSNMPTrapsCollector()
	c.services.catalog = manager
	return c
}

func newTestReverseDNSResolver() *reversedns.Resolver {
	return reversedns.New(reversedns.Config{})
}

func newTestTrapEnricher(store *ddsnmp.DeviceStore, topology enrichment.TopologyLookup, dns enrichment.ReverseDNS) *enrichment.Enricher {
	return enrichment.New(netdataadapter.RegistryLookup(store), topology, dns)
}
