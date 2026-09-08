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

func validateMetricRowShape(path string, metric *MetricsConfig) error {
	var errs []error
	scalar := isSymbolConfigured(metric.Symbol)
	if !scalar && !metric.IsColumn() {
		errs = append(errs, fmt.Errorf("%s: either a table symbol or a scalar symbol must be provided", path))
	}
	if scalar && metric.IsColumn() {
		errs = append(errs, fmt.Errorf("%s: table symbol and scalar symbol cannot be both provided", path))
	}
	if metric.IsColumn() && metric.Table.OID == "" {
		errs = append(errs, fmt.Errorf("%s.table.OID: column symbols require a table OID", path))
	}
	return errors.Join(errs...)
}

func validateEnrichMetrics(metrics []MetricsConfig) error {
	var errs []error
	for i := range metrics {
		metric := &metrics[i]
		path := fmt.Sprintf("metrics[%d]", i)
		errs = append(errs, validateMetricRowShape(path, metric))
		if isSymbolConfigured(metric.Symbol) {
			errs = append(errs, withValidationPath(path+".symbol", validateEnrichSymbol(&metric.Symbol, scalarSymbol)))
		}
		for j := range metric.Symbols {
			errs = append(
				errs,
				withValidationPath(
					fmt.Sprintf("%s.symbols[%d]", path, j),
					validateEnrichSymbol(&metric.Symbols[j], columnSymbol),
				),
			)
		}
		if metric.IsColumn() && len(metric.MetricTags) == 0 {
			errs = append(
				errs,
				fmt.Errorf(
					"%s: column symbols require at least one discriminating metric tag (such as a row index) to distinguish table rows",
					path,
				),
			)
		}
		errs = append(errs, validateEnrichMetricTags(path, metric.MetricTags, !metric.IsColumn()))
	}
	return errors.Join(errs...)
}
