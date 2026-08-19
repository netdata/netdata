// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/enrichment"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/enrichment/netdataadapter"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/telemetry"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
)

// Register registers the SNMP traps collector with shared SNMP-family enrichment state.
func Register(deviceStore *ddsnmp.DeviceStore, topologyEnricher *snmptopology.TrapEnrichmentHandle, reverseDNS *reversedns.Resolver) {
	collectorapi.Register("snmp_traps", newCreator(deviceStore, topologyEnricher, reverseDNS))
}

func newCreator(deviceStore *ddsnmp.DeviceStore, topologyEnricher *snmptopology.TrapEnrichmentHandle, reverseDNS *reversedns.Resolver) collectorapi.Creator {
	services := requiredPluginServices("Register", deviceStore, topologyEnricher, reverseDNS)
	return collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1,
		},
		CreateV2: func() collectorapi.CollectorV2 {
			services.primeCatalog()
			return newCollector(services)
		},
		Config: func() any { return &Config{} },
		AgentFunctions: func() []funcapi.FunctionConfig {
			return snmpTrapsMethods(services.journalActivity.Available)
		},
		MethodHandler: snmpTrapsMethodHandler,
	}
}

// New returns an SNMP traps collector using the provided SNMP-family enrichment state.
func New(deviceStore *ddsnmp.DeviceStore, topologyEnricher *snmptopology.TrapEnrichmentHandle, reverseDNS *reversedns.Resolver) *Collector {
	return newCollector(requiredPluginServices("New", deviceStore, topologyEnricher, reverseDNS))
}

func requiredPluginServices(caller string, deviceStore *ddsnmp.DeviceStore, topologyEnricher *snmptopology.TrapEnrichmentHandle, reverseDNS *reversedns.Resolver) *pluginServices {
	if deviceStore == nil {
		panic("snmp_traps " + caller + " requires a non-nil device store")
	}
	if topologyEnricher == nil {
		panic("snmp_traps " + caller + " requires a non-nil trap enrichment handle")
	}
	if reverseDNS == nil {
		panic("snmp_traps " + caller + " requires a non-nil reverse DNS resolver")
	}
	return newPluginServices(
		enrichment.New(
			netdataadapter.RegistryLookup(deviceStore),
			netdataadapter.TopologyLookup(topologyEnricher),
			reverseDNS,
		),
		newHostIdentityService(),
		telemetry.NewRegistry(),
		netdataEngineStateRoot,
	)
}

func newCollector(services *pluginServices) *Collector {
	return &Collector{
		Config: Config{
			Versions: []string{"v1", "v2c"},
			Listen: ListenConfig{
				ReceiveBuffer: receiver.DefaultReceiveBuffer,
			},
		},
		store:    metrix.NewCollectorStore(),
		services: services,
	}
}
