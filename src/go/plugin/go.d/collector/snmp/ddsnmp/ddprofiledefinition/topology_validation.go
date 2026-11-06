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
		path := fmt.Sprintf("topology[%d]", i)
		errs = append(errs, validateMetricRowShape(path, metricConfig))
		if isSymbolConfigured(metricConfig.Symbol) {
			errs = append(
				errs,
				validateEnrichTopologySymbol(path+".symbol", &metricConfig.Symbol, topologyScalarSymbol),
			)
		}
		for j := range metricConfig.Symbols {
			errs = append(
				errs,
				validateEnrichTopologySymbol(
					fmt.Sprintf("%s.symbols[%d]", path, j),
					&metricConfig.Symbols[j],
					topologyColumnSymbol,
				),
			)
		}
		errs = append(errs, validateEnrichMetricTags(path, metricConfig.MetricTags, !metricConfig.IsColumn()))
	}

	return errors.Join(errs...)
}

func validateEnrichTopologySymbol(path string, symbol *SymbolConfig, context symbolContext) error {
	var errs []error

	errs = append(errs, withValidationPath(path, validateEnrichSymbol(symbol, context)))
	if strings.HasPrefix(symbol.Name, "_") {
		errs = append(
			errs,
			fmt.Errorf("%s: symbol name %q cannot be underscore-prefixed", path, symbol.Name),
		)
	}
	if symbol.ChartMeta != (ChartMeta{}) {
		errs = append(errs, fmt.Errorf("%s: chart_meta cannot be used in topology rows", path))
	}
	if symbol.MetricType != "" {
		errs = append(errs, fmt.Errorf("%s: metric_type cannot be used in topology rows", path))
	}
	if symbol.Mapping.HasItems() || symbol.Mapping.Mode != "" {
		errs = append(errs, fmt.Errorf("%s: mapping cannot be used in topology rows", path))
	}
	if symbol.Transform != "" {
		errs = append(errs, fmt.Errorf("%s: transform cannot be used in topology rows", path))
	}
	if symbol.ScaleFactor != 0 {
		errs = append(errs, fmt.Errorf("%s: scale_factor cannot be used in topology rows", path))
	}
	if symbol.Format != "" {
		errs = append(errs, fmt.Errorf("%s: format cannot be used in topology rows", path))
	}
	if context == topologyColumnSymbol {
		if symbol.ExtractValue != "" {
			errs = append(
				errs,
				fmt.Errorf("%s: extract_value cannot be used on table presence symbols", path),
			)
		}
		if symbol.MatchPattern != "" {
			errs = append(
				errs,
				fmt.Errorf("%s: match_pattern cannot be used on table presence symbols", path),
			)
		}
		if symbol.MatchValue != "" {
			errs = append(
				errs,
				fmt.Errorf("%s: match_value cannot be used on table presence symbols", path),
			)
		}
	}

	return errors.Join(errs...)
}
