// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type topologyMetricLookupKey struct {
	table string
	name  string
}

func (c *Collector) collectTopologyMetrics(prof *ddsnmp.Profile, stats *ddsnmp.CollectionStats) ([]ddsnmp.Metric, error) {
	if prof.Definition == nil || len(prof.Definition.Topology) == 0 {
		return nil, nil
	}

	metricsConfig, kinds := topologyRowsAsMetrics(prof.Definition.Topology)
	topologyProfile := &ddsnmp.Profile{
		SourceFile: prof.SourceFile,
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: metricsConfig,
		},
	}

	scalarMetrics, err := c.scalarCollector.collect(topologyProfile, stats)
	if err != nil {
		return nil, err
	}
	tableMetrics, err := c.tableCollector.collect(topologyProfile, stats)
	if err != nil {
		return nil, err
	}

	topologyMetrics := append(scalarMetrics, tableMetrics...)
	for i := range topologyMetrics {
		key := topologyMetricLookupKey{table: topologyMetrics[i].Table, name: topologyMetrics[i].Name}
		topologyMetrics[i].TopologyKind = kinds[key]
	}

	return topologyMetrics, nil
}

func topologyRowsAsMetrics(topology []ddprofiledefinition.TopologyConfig) ([]ddprofiledefinition.MetricsConfig, map[topologyMetricLookupKey]ddprofiledefinition.TopologyKind) {
	metrics := make([]ddprofiledefinition.MetricsConfig, 0, len(topology))
	kinds := make(map[topologyMetricLookupKey]ddprofiledefinition.TopologyKind)

	for _, topo := range topology {
		cfg := topo.MetricsConfig
		metrics = append(metrics, cfg)
		switch {
		case cfg.IsScalar():
			kinds[topologyMetricLookupKey{name: cfg.Symbol.Name}] = topo.Kind
		case cfg.IsColumn():
			for _, sym := range cfg.Symbols {
				kinds[topologyMetricLookupKey{table: cfg.Table.Name, name: sym.Name}] = topo.Kind
			}
		}
	}

	return metrics, kinds
}
