// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"errors"
	"fmt"
)

// normalizeMetrics moves legacy row fields into the symbols consumed by collection.
func normalizeMetrics(metrics []MetricsConfig) {
	for i := range metrics {
		normalizeMetric(&metrics[i])
	}
}

func normalizeMetric(metric *MetricsConfig) {
	if metric.Symbol.Name == "" && metric.Symbol.OID == "" && metric.Name != "" && metric.OID != "" {
		metric.Symbol.Name = metric.Name
		metric.Symbol.OID = metric.OID
		metric.Name = ""
		metric.OID = ""
	}
	// Normalize the row default into the symbols consumed by collection.
	if metric.IsColumn() {
		for i := range metric.Symbols {
			if metric.Symbols[i].MetricType == "" {
				metric.Symbols[i].MetricType = metric.MetricType
			}
		}
	} else if metric.Symbol.MetricType == "" {
		metric.Symbol.MetricType = metric.MetricType
	}
	metric.MetricType = ""
}

func validateEnrichMetrics(metrics []MetricsConfig) error {
	var errs []error

	for i := range metrics {
		metricConfig := &metrics[i]
		if !metricConfig.IsScalar() && !metricConfig.IsColumn() {
			errs = append(
				errs,
				fmt.Errorf("either a table symbol or a scalar symbol must be provided: %#v", metricConfig),
			)
		}
		if metricConfig.IsScalar() && metricConfig.IsColumn() {
			errs = append(errs, fmt.Errorf("table symbol and scalar symbol cannot be both provided: %#v", metricConfig))
		}
		if metricConfig.IsScalar() {
			errs = append(errs, validateEnrichSymbol(&metricConfig.Symbol, scalarSymbol))
			for j := range metricConfig.MetricTags {
				metricTag := &metricConfig.MetricTags[j]
				errs = append(errs, validateEnrichMetricTag(metricTag))
				errs = append(errs, validateScalarMetricTag("", metricTag))
			}
		}
		if metricConfig.IsColumn() {
			for j := range metricConfig.Symbols {
				errs = append(errs, validateEnrichSymbol(&metricConfig.Symbols[j], columnSymbol))
			}
			if len(metricConfig.MetricTags) == 0 {
				errs = append(
					errs,
					fmt.Errorf(
						"column symbols doesn't have a 'metric_tags' section (%+v), all its metrics will use the same tags; "+
							"if the table has multiple rows, only one row will be submitted; "+
							"please add at least one discriminating metric tag (such as a row index) "+
							"to ensure metrics of all rows are submitted",
						metricConfig.Symbols,
					),
				)
			}
			for i := range metricConfig.MetricTags {
				metricTag := &metricConfig.MetricTags[i]
				errs = append(errs, validateEnrichMetricTag(metricTag))
			}
		}
	}

	return errors.Join(errs...)
}
