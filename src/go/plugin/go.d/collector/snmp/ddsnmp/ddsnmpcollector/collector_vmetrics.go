// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"slices"

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

type (
	// vmetricsSourceKey identifies a metric source
	vmetricsSourceKey struct {
		metricName string
		tableName  string
	}
	// which aggregator to feed and under which dimension name
	vmetricsSink struct {
		agg *vmetricsAggregator
		dim string // empty == non-composite (single-source behavior)
	}
	// vmetricsAggregator holds accumulation state for a virtual metric
	vmetricsAggregator struct {
		config      ddprofiledefinition.VirtualMetricConfig
		sum         int64
		multiSum    map[string]int64 // for aggregating MultiValue metrics
		perDim      map[string]int64 // per-source accumulation when composite
		sourceCount int
		metricType  ddprofiledefinition.ProfileMetricType
	}
)

func (p *vmetricsCollector) Collect(profDef *ddprofiledefinition.ProfileDefinition, collectedMetrics []ddsnmp.Metric) []ddsnmp.Metric {
	if len(profDef.VirtualMetrics) == 0 {
		return nil
	}

	sourceToAggregators, aggregators := p.buildAggregators(profDef)

	for _, metric := range collectedMetrics {
		key := vmetricsSourceKey{
			metricName: metric.Name,
			tableName:  metric.Table,
		}

		sinks, found := sourceToAggregators[key]
		if !found {
			continue
		}

		for _, sink := range sinks {
			if sink.agg == nil {
				continue
			}

			agg := sink.agg

			// If sink.dim != "" => composite path (this VM has multiple sources)
			if sink.dim != "" {
				// We need a single number per source (dimension).
				// If the incoming base metric is MultiValue, collapse it to a total; else use Value.
				var v int64
				if len(metric.MultiValue) > 0 {
					for _, mv := range metric.MultiValue {
						v += mv
					}
				} else {
					v = metric.Value
				}

				if agg.perDim == nil {
					agg.perDim = make(map[string]int64)
				}
				agg.perDim[sink.dim] += v
			} else {
				// Non-composite (single-source) => keep existing behavior:
				if len(metric.MultiValue) > 0 {
					if agg.multiSum == nil {
						agg.multiSum = make(map[string]int64)
					}
					for state, value := range metric.MultiValue {
						agg.multiSum[state] += value
					}
				} else {
					agg.sum += metric.Value
				}
			}

			agg.sourceCount++
			if agg.metricType == "" {
				agg.metricType = metric.MetricType
			} else if agg.metricType != metric.MetricType {
				p.log.Debugf("virtual metric %q mixes MetricType (%s vs %s); using %s",
					agg.config.Name, agg.metricType, metric.MetricType, agg.metricType)
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

		vm := ddsnmp.Metric{
			Name:        agg.config.Name,
			Description: agg.config.ChartMeta.Description,
			Family:      agg.config.ChartMeta.Family,
			Unit:        agg.config.ChartMeta.Unit,
			ChartType:   agg.config.ChartMeta.Type,
			MetricType:  agg.metricType,
		}

		switch {
		case len(agg.perDim) > 0:
			// Composite output: one metric with MultiValue where keys are dimension names (sources)
			vm.MultiValue = agg.perDim
		case len(agg.multiSum) > 0:
			// Single-source whose base metric was MultiValue
			vm.MultiValue = agg.multiSum
		default:
			// Single-source
			vm.Value = agg.sum
		}
		virtualMetrics = append(virtualMetrics, vm)
	}

	return virtualMetrics
}

func (p *vmetricsCollector) buildAggregators(profDef *ddprofiledefinition.ProfileDefinition) (map[vmetricsSourceKey][]vmetricsSink, []*vmetricsAggregator) {
	sourceToAggregators := make(map[vmetricsSourceKey][]vmetricsSink)
	aggregators := make([]*vmetricsAggregator, 0, len(profDef.VirtualMetrics))

	existingNames := p.getDefinedMetricNames(profDef.Metrics)

	for _, config := range profDef.VirtualMetrics {
		if existingNames[config.Name] {
			p.log.Warningf("virtual metric '%s' conflicts with existing metric, skipping", config.Name)
			continue
		}

		agg := &vmetricsAggregator{config: config}
		aggregators = append(aggregators, agg)

		isComposite := len(config.Sources) > 1 &&
			slices.ContainsFunc(config.Sources, func(s ddprofiledefinition.VirtualMetricSourceConfig) bool {
				return s.As != ""
			})

		// Register this aggregator for each source it needs
		for _, source := range config.Sources {
			key := vmetricsSourceKey{
				metricName: source.Metric,
				tableName:  source.Table,
			}

			var dim string
			if isComposite {
				dim = ternary(source.As != "", source.As, source.Metric)
			}

			sourceToAggregators[key] = append(sourceToAggregators[key], vmetricsSink{
				agg: agg,
				dim: dim,
			})
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
