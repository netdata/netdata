// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type vmetricsCollector struct {
	log *logger.Logger
}

func newVirtualMetricsCollector(log *logger.Logger) *vmetricsCollector {
	return &vmetricsCollector{
		log: log,
	}
}

// vmetricsSourceKey identifies a metric source
type vmetricsSourceKey struct {
	metricName string
	tableName  string
}

// vmetricsAggregator holds accumulation state for a virtual metric
type vmetricsAggregator struct {
	config      ddprofiledefinition.VirtualMetricConfig
	sum         int64
	sourceCount int
	metricType  ddprofiledefinition.ProfileMetricType
}

func (p *vmetricsCollector) Collect(profDef *ddprofiledefinition.ProfileDefinition, collectedMetrics []ddsnmp.Metric) []ddsnmp.Metric {
	if len(profDef.VirtualMetrics) == 0 {
		return nil
	}

	sourceToAggregators, aggregators := p.buildAggregators(profDef)

	for _, metric := range collectedMetrics {
		if !metric.IsTable || metric.Table == "" {
			continue
		}

		key := vmetricsSourceKey{
			metricName: metric.Name,
			tableName:  metric.Table,
		}

		// Find all aggregators that need this metric
		if aggrs, found := sourceToAggregators[key]; found {
			for _, agg := range aggrs {
				agg.sum += metric.Value
				agg.sourceCount++
				if agg.metricType == "" {
					agg.metricType = metric.MetricType
				}
			}
		}
	}

	// Build virtual metrics from aggregators
	var virtualMetrics []ddsnmp.Metric
	for _, agg := range aggregators {
		if agg.sourceCount == 0 {
			p.log.Debugf("no source metrics found for virtual metric '%s'", agg.config.Name)
			continue
		}

		virtualMetrics = append(virtualMetrics, ddsnmp.Metric{
			Name:        agg.config.Name,
			Value:       agg.sum,
			Description: agg.config.ChartMeta.Description,
			Family:      agg.config.ChartMeta.Family,
			Unit:        agg.config.ChartMeta.Unit,
			MetricType:  agg.metricType,
		})
	}

	return virtualMetrics
}

func (p *vmetricsCollector) buildAggregators(profDef *ddprofiledefinition.ProfileDefinition) (map[vmetricsSourceKey][]*vmetricsAggregator, []*vmetricsAggregator) {
	sourceToAggregators := make(map[vmetricsSourceKey][]*vmetricsAggregator)
	aggregators := make([]*vmetricsAggregator, 0, len(profDef.VirtualMetrics))

	existingNames := p.getDefinedMetricNames(profDef.Metrics)

	for _, config := range profDef.VirtualMetrics {
		if existingNames[config.Name] {
			p.log.Warningf("virtual metric '%s' conflicts with existing metric, skipping", config.Name)
			continue
		}

		agg := &vmetricsAggregator{
			config: config,
		}
		aggregators = append(aggregators, agg)

		// Register this aggregator for each source it needs
		for _, source := range config.Sources {
			if source.Table == "" {
				p.log.Warningf("virtual metric '%s' source '%s' missing table, skipping source", config.Name, source.Metric)
				continue
			}

			key := vmetricsSourceKey{
				metricName: source.Metric,
				tableName:  source.Table,
			}

			sourceToAggregators[key] = append(sourceToAggregators[key], agg)
		}
	}

	return sourceToAggregators, aggregators
}

func (p *vmetricsCollector) getDefinedMetricNames(profMetrics []ddprofiledefinition.MetricsConfig) map[string]bool {
	names := make(map[string]bool)
	for _, m := range profMetrics {
		switch {
		case m.IsScalar():
			names[m.Symbol.Name] = true
		case m.IsColumn():
			for _, sym := range m.Symbols {
				names[sym.Name] = true
			}
		}
	}
	return names
}
