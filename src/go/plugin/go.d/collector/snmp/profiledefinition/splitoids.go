// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

import "sort"

// splitOIDs returns all scalar and column (i.e. table) OIDs from metrics, tags, and metadata
func splitOIDs(metrics []MetricsConfig, globalTags []MetricTagConfig, metadata MetadataConfig) ([]string, []string) {
	scalars := make(map[string]bool)
	columns := make(map[string]bool)
	// Singular metric values are scalars; metrics with .Symbols are tables,
	// and their symbols and tags are both expected to be columns.
	for _, metric := range metrics {
		scalars[metric.Symbol.OID] = true
		for _, symbolConfig := range metric.Symbols {
			columns[symbolConfig.OID] = true
		}
		for _, metricTag := range metric.MetricTags {
			columns[metricTag.Symbol.OID] = true
		}
	}
	// Global tags are scalar by definition
	for _, tag := range globalTags {
		scalars[tag.Symbol.OID] = true
	}
	// Metadata fields are all columns except when IsMetadataResourceWithScalarOids is true
	for resource, metadataConfig := range metadata {
		target := columns
		if IsMetadataResourceWithScalarOids(resource) {
			target = scalars
		}
		for _, field := range metadataConfig.Fields {
			target[field.Symbol.OID] = true
			for _, symbol := range field.Symbols {
				target[symbol.OID] = true
			}
		}
		for _, tagConfig := range metadataConfig.IDTags {
			target[tagConfig.Symbol.OID] = true
		}
	}
	scalarValues := make([]string, 0, len(scalars))
	for key := range scalars {
		if key == "" {
			continue
		}
		scalarValues = append(scalarValues, key)
	}
	columnValues := make([]string, 0, len(columns))
	for key := range columns {
		if key == "" {
			continue
		}
		columnValues = append(columnValues, key)
	}
	// Sort them for deterministic testing
	sort.Strings(scalarValues)
	sort.Strings(columnValues)
	return scalarValues, columnValues
}
