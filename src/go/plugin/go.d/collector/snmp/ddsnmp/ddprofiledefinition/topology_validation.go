// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"errors"
	"fmt"
	"strings"
)

func normalizeTopology(topology []TopologyConfig) {
	for i := range topology {
		normalizeMetric(&topology[i].MetricsConfig)
	}
}

func validateEnrichTopology(topology []TopologyConfig) error {
	var errs []error

	for i := range topology {
		topo := &topology[i]
		metricConfig := &topo.MetricsConfig

		if topo.Kind == "" {
			errs = append(errs, fmt.Errorf("topology[%d]: missing kind", i))
		} else if !IsValidTopologyKind(topo.Kind) {
			errs = append(errs, fmt.Errorf("topology[%d]: invalid kind %q", i, topo.Kind))
		}
		if !metricConfig.IsScalar() && !metricConfig.IsColumn() {
			errs = append(
				errs,
				fmt.Errorf(
					"topology[%d]: either a table symbol or a scalar symbol must be provided: %#v",
					i,
					metricConfig,
				),
			)
		}
		if metricConfig.IsScalar() && metricConfig.IsColumn() {
			errs = append(
				errs,
				fmt.Errorf(
					"topology[%d]: table symbol and scalar symbol cannot be both provided: %#v",
					i,
					metricConfig,
				),
			)
		}
		if metricConfig.IsScalar() {
			errs = append(errs, validateEnrichTopologySymbol(i, &metricConfig.Symbol, topologyScalarSymbol))
			for j := range metricConfig.MetricTags {
				metricTag := &metricConfig.MetricTags[j]
				errs = append(errs, validateEnrichMetricTag(metricTag))
				errs = append(
					errs,
					validateScalarMetricTag(fmt.Sprintf("topology[%d].metric_tags[%d]", i, j), metricTag),
				)
			}
		}
		if metricConfig.IsColumn() {
			for j := range metricConfig.Symbols {
				errs = append(errs, validateEnrichTopologySymbol(i, &metricConfig.Symbols[j], topologyColumnSymbol))
			}
			for j := range metricConfig.MetricTags {
				errs = append(errs, validateEnrichMetricTag(&metricConfig.MetricTags[j]))
			}
		}
	}

	return errors.Join(errs...)
}

func validateEnrichTopologySymbol(topologyIdx int, symbol *SymbolConfig, context symbolContext) error {
	var errs []error

	errs = append(errs, validateEnrichSymbol(symbol, context))
	if strings.HasPrefix(symbol.Name, "_") {
		errs = append(
			errs,
			fmt.Errorf("topology[%d]: symbol name %q cannot be underscore-prefixed", topologyIdx, symbol.Name),
		)
	}
	if symbol.ChartMeta != (ChartMeta{}) {
		errs = append(errs, fmt.Errorf("topology[%d]: chart_meta cannot be used in topology rows", topologyIdx))
	}
	if symbol.MetricType != "" {
		errs = append(errs, fmt.Errorf("topology[%d]: metric_type cannot be used in topology rows", topologyIdx))
	}
	if symbol.Mapping.HasItems() || symbol.Mapping.Mode != "" {
		errs = append(errs, fmt.Errorf("topology[%d]: mapping cannot be used in topology rows", topologyIdx))
	}
	if symbol.Transform != "" {
		errs = append(errs, fmt.Errorf("topology[%d]: transform cannot be used in topology rows", topologyIdx))
	}
	if symbol.ScaleFactor != 0 {
		errs = append(errs, fmt.Errorf("topology[%d]: scale_factor cannot be used in topology rows", topologyIdx))
	}
	if symbol.Format != "" {
		errs = append(errs, fmt.Errorf("topology[%d]: format cannot be used in topology rows", topologyIdx))
	}
	if symbol.ConstantValueOne {
		errs = append(errs, fmt.Errorf("topology[%d]: constant_value_one cannot be used in topology rows", topologyIdx))
	}
	if context == topologyColumnSymbol {
		if symbol.ExtractValue != "" {
			errs = append(
				errs,
				fmt.Errorf("topology[%d]: extract_value cannot be used on table presence symbols", topologyIdx),
			)
		}
		if symbol.MatchPattern != "" {
			errs = append(
				errs,
				fmt.Errorf("topology[%d]: match_pattern cannot be used on table presence symbols", topologyIdx),
			)
		}
		if symbol.MatchValue != "" {
			errs = append(
				errs,
				fmt.Errorf("topology[%d]: match_value cannot be used on table presence symbols", topologyIdx),
			)
		}
	}

	return errors.Join(errs...)
}
