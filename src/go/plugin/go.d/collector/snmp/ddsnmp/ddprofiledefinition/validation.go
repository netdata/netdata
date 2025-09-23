// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
)

var validMetadataResources = map[string]map[string]bool{
	"device": {
		"name":          true,
		"description":   true,
		"sys_object_id": true,
		"location":      true,
		"serial_number": true,
		"vendor":        true,
		"version":       true,
		"product_name":  true,
		"model":         true,
		"os_name":       true,
		"os_version":    true,
		"os_hostname":   true,
		"type":          true,
	},
	"interface": {
		"name":         true,
		"alias":        true,
		"description":  true,
		"mac_address":  true,
		"admin_status": true,
		"oper_status":  true,
	},
}

// SymbolContext represent the context in which the symbol is used
type SymbolContext int64

// ScalarSymbol enums
const (
	ScalarSymbol SymbolContext = iota
	ColumnSymbol
	MetricTagSymbol
	MetadataSymbol
)

// ValidateEnrichProfile validates a profile and normalizes it.
func ValidateEnrichProfile(p *ProfileDefinition) error {
	NormalizeMetrics(p.Metrics)

	var errs []error

	if err := validateEnrichLegacySelector(p); err != nil {
		errs = append(errs, err)
	}
	for _, err := range ValidateEnrichMetadata(p.Metadata) {
		errs = append(errs, errors.New(err))
	}
	for _, err := range ValidateEnrichSysobjectIDMetadata(p.SysobjectIDMetadata) {
		errs = append(errs, errors.New(err))
	}
	for _, err := range ValidateEnrichMetrics(p.Metrics) {
		errs = append(errs, errors.New(err))
	}
	for _, err := range ValidateEnrichMetricTags(p.MetricTags) {
		errs = append(errs, errors.New(err))
	}

	return errors.Join(errs...)
}

// ValidateEnrichMetricTags validates and normalizes metric tags
func ValidateEnrichMetricTags(metricTags []MetricTagConfig) []string {
	var errs []string
	for i := range metricTags {
		errs = append(errs, validateEnrichMetricTag(&metricTags[i])...)
	}
	return errs
}

// ValidateEnrichMetrics will validate MetricsConfig and enrich it.
// Example of enrichment:
// - storage of compiled regex pattern
func ValidateEnrichMetrics(metrics []MetricsConfig) []string {
	var errs []string
	for i := range metrics {
		metricConfig := &metrics[i]
		if !metricConfig.IsScalar() && !metricConfig.IsColumn() {
			errs = append(errs, fmt.Sprintf("either a table symbol or a scalar symbol must be provided: %#v", metricConfig))
		}
		if metricConfig.IsScalar() && metricConfig.IsColumn() {
			errs = append(errs, fmt.Sprintf("table symbol and scalar symbol cannot be both provided: %#v", metricConfig))
		}
		if metricConfig.IsScalar() {
			errs = append(errs, validateEnrichSymbol(&metricConfig.Symbol, ScalarSymbol)...)
		}
		if metricConfig.IsColumn() {
			for j := range metricConfig.Symbols {
				errs = append(errs, validateEnrichSymbol(&metricConfig.Symbols[j], ColumnSymbol)...)
			}
			if len(metricConfig.MetricTags) == 0 {
				errs = append(errs, fmt.Sprintf("column symbols doesn't have a 'metric_tags' section (%+v), all its metrics will use the same tags; "+
					"if the table has multiple rows, only one row will be submitted; "+
					"please add at least one discriminating metric tag (such as a row index) "+
					"to ensure metrics of all rows are submitted", metricConfig.Symbols))
			}
			for i := range metricConfig.MetricTags {
				metricTag := &metricConfig.MetricTags[i]
				errs = append(errs, validateEnrichMetricTag(metricTag)...)
			}
		}
		// Setting forced_type value to metric_type value for backward compatibility
		if metricConfig.MetricType == "" && metricConfig.ForcedType != "" {
			metricConfig.MetricType = metricConfig.ForcedType
		}
		metricConfig.ForcedType = ""
	}
	return errs
}

// ValidateEnrichMetadata will validate MetadataConfig and enrich it.
func ValidateEnrichMetadata(metadata MetadataConfig) []string {
	var errs []string
	for resName := range metadata {
		_, isValidRes := validMetadataResources[resName]
		if !isValidRes {
			errs = append(errs, fmt.Sprintf("invalid resource: %s", resName))
		} else {
			res := metadata[resName]
			for fieldName := range res.Fields {
				_, isValidField := validMetadataResources[resName][fieldName]
				if !isValidField {
					errs = append(errs, fmt.Sprintf("invalid resource (%s) field: %s", resName, fieldName))
					continue
				}
				field := res.Fields[fieldName]
				for i := range field.Symbols {
					errs = append(errs, validateEnrichSymbol(&field.Symbols[i], MetadataSymbol)...)
				}
				if field.Symbol.OID != "" {
					errs = append(errs, validateEnrichSymbol(&field.Symbol, MetadataSymbol)...)
				}
				res.Fields[fieldName] = field
			}
			metadata[resName] = res
		}
		if resName == "device" && len(metadata[resName].IDTags) > 0 {
			errs = append(errs, "device resource does not support custom id_tags")
		}
		for i := range metadata[resName].IDTags {
			metricTag := &metadata[resName].IDTags[i]
			errs = append(errs, validateEnrichMetricTag(metricTag)...)
		}
	}
	return errs
}

func ValidateEnrichSysobjectIDMetadata(entries []SysobjectIDMetadataEntryConfig) []string {
	var errs []string

	// Track seen sysobjectids to detect duplicates
	seenOIDs := make(map[string]int) // OID -> first occurrence index

	for i, entry := range entries {
		// Validate sysobjectid is not empty
		if entry.SysobjectID == "" {
			errs = append(errs, fmt.Sprintf("sysobjectid_metadata[%d]: missing sysobjectid", i))
			continue
		}

		// Check for duplicate sysobjectids
		if firstIdx, exists := seenOIDs[entry.SysobjectID]; exists {
			errs = append(errs, fmt.Sprintf("sysobjectid_metadata[%d]: duplicate sysobjectid %s (first occurrence at index %d)",
				i, entry.SysobjectID, firstIdx))
		} else {
			seenOIDs[entry.SysobjectID] = i
		}

		// Validate metadata fields
		for fieldName, field := range entry.Metadata {
			// Validate the field must have either value or symbol(s)
			if field.Value == "" && field.Symbol.OID == "" && len(field.Symbols) == 0 {
				errs = append(errs, fmt.Sprintf("sysobjectid_metadata[%d].%s: must have either value or symbol(s)", i, fieldName))
			}

			// Can't have both value and symbols
			if field.Value != "" && (field.Symbol.OID != "" || len(field.Symbols) > 0) {
				errs = append(errs, fmt.Sprintf("sysobjectid_metadata[%d].%s: cannot have both value and symbol(s)", i, fieldName))
			}

			// Validate symbols if present
			for j := range field.Symbols {
				symbolErrors := validateEnrichSymbol(&field.Symbols[j], MetadataSymbol)
				for _, err := range symbolErrors {
					errs = append(errs, fmt.Sprintf("sysobjectid_metadata[%d].%s.symbols[%d]: %s", i, fieldName, j, err))
				}
			}

			// Validate single symbol if present
			if field.Symbol.OID != "" {
				symbolErrors := validateEnrichSymbol(&field.Symbol, MetadataSymbol)
				for _, err := range symbolErrors {
					errs = append(errs, fmt.Sprintf("sysobjectid_metadata[%d].%s.symbol: %s", i, fieldName, err))
				}
			}

			entry.Metadata[fieldName] = field
		}
	}

	return errs
}

func validateEnrichSymbol(symbol *SymbolConfig, symbolContext SymbolContext) []string {
	var errs []string
	if symbol.Name == "" {
		errs = append(errs, fmt.Sprintf("symbol name missing: name=`%s` oid=`%s`", symbol.Name, symbol.OID))
	}
	if symbol.OID == "" {
		if symbolContext == ColumnSymbol && !symbol.ConstantValueOne {
			errs = append(errs, fmt.Sprintf("symbol oid or send_as_one missing: name=`%s` oid=`%s`", symbol.Name, symbol.OID))
		} else if symbolContext != ColumnSymbol {
			errs = append(errs, fmt.Sprintf("symbol oid missing: name=`%s` oid=`%s`", symbol.Name, symbol.OID))
		}
	}
	if symbol.ExtractValue != "" {
		pattern, err := regexp.Compile(symbol.ExtractValue)
		if err != nil {
			errs = append(errs, fmt.Sprintf("cannot compile `extract_value` (%s): %s", symbol.ExtractValue, err.Error()))
		} else {
			symbol.ExtractValueCompiled = pattern
		}
	}
	if symbol.MatchPattern != "" {
		pattern, err := regexp.Compile(symbol.MatchPattern)
		if err != nil {
			errs = append(errs, fmt.Sprintf("cannot compile `extract_value` (%s): %s", symbol.ExtractValue, err.Error()))
		} else {
			symbol.MatchPatternCompiled = pattern
		}
	}
	if symbolContext != ColumnSymbol && symbol.ConstantValueOne {
		errs = append(errs, "`constant_value_one` cannot be used outside of tables")
	}
	if (symbolContext != ColumnSymbol && symbolContext != ScalarSymbol) && symbol.MetricType != "" {
		errs = append(errs, "`metric_type` cannot be used outside scalar/table metric symbols and metrics root")
	}
	return errs
}

func validateEnrichMetricTag(metricTag *MetricTagConfig) []string {
	var errs []string
	if (metricTag.Column.OID != "" || metricTag.Column.Name != "") && (metricTag.Symbol.OID != "" || metricTag.Symbol.Name != "") {
		errs = append(errs, fmt.Sprintf("metric tag symbol and column cannot be both declared: symbol=%v, column=%v", metricTag.Symbol, metricTag.Column))
	}

	// Move deprecated metricTag.Column to metricTag.Symbol
	if metricTag.Column.OID != "" || metricTag.Column.Name != "" {
		metricTag.Symbol = SymbolConfigCompat(metricTag.Column)
		metricTag.Column = SymbolConfig{}
	}

	// OID/Name to Symbol harmonization:
	// When users declare metric tag like:
	//   metric_tags:
	//     - OID: 1.2.3
	//       symbol: aSymbol
	// this will lead to OID stored as MetricTagConfig.OID  and name stored as MetricTagConfig.Symbol.Name
	// When this happens, we harmonize by moving MetricTagConfig.OID to MetricTagConfig.Symbol.OID.
	if metricTag.OID != "" && metricTag.Symbol.OID != "" {
		errs = append(errs, fmt.Sprintf("metric tag OID and symbol.OID cannot be both declared: OID=%s, symbol.OID=%s", metricTag.OID, metricTag.Symbol.OID))
	}
	if metricTag.OID != "" && metricTag.Symbol.OID == "" {
		metricTag.Symbol.OID = metricTag.OID
		metricTag.OID = ""
	}
	if metricTag.Symbol.OID != "" || metricTag.Symbol.Name != "" {
		symbol := SymbolConfig(metricTag.Symbol)
		errs = append(errs, validateEnrichSymbol(&symbol, MetricTagSymbol)...)
		metricTag.Symbol = SymbolConfigCompat(symbol)
	}
	if metricTag.Match != "" {
		pattern, err := regexp.Compile(metricTag.Match)
		if err != nil {
			errs = append(errs, fmt.Sprintf("cannot compile `match` (`%s`): %s", metricTag.Match, err.Error()))
		} else {
			metricTag.Pattern = pattern
		}
		if len(metricTag.Tags) == 0 {
			errs = append(errs, fmt.Sprintf("`tags` mapping must be provided if `match` (`%s`) is defined", metricTag.Match))
		}
	}
	if len(metricTag.Mapping) > 0 && metricTag.Tag == "" {
		errs = append(errs, fmt.Sprintf("``tag` must be provided if `mapping` (`%s`) is defined", metricTag.Mapping))
	}
	for _, transform := range metricTag.IndexTransform {
		if transform.Start > transform.End {
			errs = append(errs, fmt.Sprintf("transform rule end should be greater than start. Invalid rule: %#v", transform))
		}
	}
	return errs
}

// validateEnrichLegacySelector migrates legacy sysobjectid into selector include/exclude
// and validates that all OID patterns compile as regex.
func validateEnrichLegacySelector(p *ProfileDefinition) error {
	var errs []error

	// If the new selector is absent but legacy sysobjectid exists, migrate it.
	if len(p.Selector) == 0 && len(p.SysObjectIDs) > 0 {
		p.Selector = SelectorSpec{
			{
				SysObjectID: SelectorIncludeExclude{
					Include: slices.Clone(p.SysObjectIDs), // legacy -> include
				},
			},
		}
	}

	// Normalize and validate every rule
	for i := range p.Selector {
		r := &p.Selector[i]

		// Validate regex syntax for sysObjectID includes/excludes
		for j, pat := range r.SysObjectID.Include {
			if _, err := regexp.Compile(pat); err != nil {
				errs = append(errs, fmt.Errorf("selector[%d].sysObjectID.include[%d]: invalid regex %q: %v", i, j, pat, err))
			}
		}
		for j, pat := range r.SysObjectID.Exclude {
			if _, err := regexp.Compile(pat); err != nil {
				errs = append(errs, fmt.Errorf("selector[%d].sysObjectID.exclude[%d]: invalid regex %q: %v", i, j, pat, err))
			}
		}
	}

	return errors.Join(errs...)
}
