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
	"strconv"
	"strings"
)

var validMetadataResources = map[string]map[string]bool{
	"device": {
		"name":                        true,
		"description":                 true,
		"sys_object_id":               true,
		"location":                    true,
		"serial_number":               true,
		"vendor":                      true,
		"version":                     true,
		"software_version":            true,
		"firmware_version":            true,
		"hardware_version":            true,
		"product_name":                true,
		"model":                       true,
		"os_name":                     true,
		"os_version":                  true,
		"os_hostname":                 true,
		"category":                    true,
		"type":                        true,
		"lldp_loc_chassis_id":         true,
		"lldp_loc_chassis_id_subtype": true,
		"lldp_loc_sys_name":           true,
		"lldp_loc_sys_desc":           true,
		"lldp_loc_sys_cap_supported":  true,
		"lldp_loc_sys_cap_enabled":    true,
		"bridge_base_address":         true,
		"stp_designated_root":         true,
		"vtp_version":                 true,
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
	normalizeMetrics(p.Metrics)

	errs := []error{
		validateEnrichLegacySelector(p),
		validateEnrichMetadata(p.Metadata),
		validateEnrichSysobjectIDMetadata(p.SysobjectIDMetadata),
		validateEnrichMetrics(p.Metrics),
		validateEnrichMetricTags(p.MetricTags),
		validateEnrichVirtualMetrics(p.Metrics, p.VirtualMetrics),
	}

	return errors.Join(errs...)
}

// normalizeMetrics converts legacy syntax to new syntax
// 1/ converts old symbol syntax to new symbol syntax
// metric.Name and metric.OID info are moved to metric.Symbol.Name and metric.Symbol.OID
func normalizeMetrics(metrics []MetricsConfig) {
	for i := range metrics {
		metric := &metrics[i]

		// converts old symbol syntax to new symbol syntax
		if metric.Symbol.Name == "" && metric.Symbol.OID == "" && metric.Name != "" && metric.OID != "" {
			metric.Symbol.Name = metric.Name
			metric.Symbol.OID = metric.OID
			metric.Name = ""
			metric.OID = ""
		}
	}
}

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

func validateEnrichMetadata(metadata MetadataConfig) error {
	var errs []error

	for resName := range metadata {
		_, isValidRes := validMetadataResources[resName]
		if !isValidRes {
			errs = append(errs, fmt.Errorf("invalid resource: %s", resName))
		} else {
			res := metadata[resName]
			for fieldName := range res.Fields {
				if !isValidMetadataField(resName, fieldName) {
					errs = append(errs, fmt.Errorf("invalid resource (%s) field: %s", resName, fieldName))
					continue
				}
				field := res.Fields[fieldName]
				for i := range field.Symbols {
					errs = append(errs, validateEnrichSymbol(&field.Symbols[i], MetadataSymbol))
				}
				if field.Symbol.OID != "" {
					errs = append(errs, validateEnrichSymbol(&field.Symbol, MetadataSymbol))
				}
				res.Fields[fieldName] = field
			}
			metadata[resName] = res
		}
		if resName == "device" && len(metadata[resName].IDTags) > 0 {
			errs = append(errs, errors.New("device resource does not support custom id_tags"))
		}
		for i := range metadata[resName].IDTags {
			metricTag := &metadata[resName].IDTags[i]
			errs = append(errs, validateEnrichMetricTag(metricTag))
		}
	}

	return errors.Join(errs...)
}

func validateEnrichSysobjectIDMetadata(entries []SysobjectIDMetadataEntryConfig) error {
	var errs []error

	// Track seen sysobjectids to detect duplicates
	seenOIDs := make(map[string]int) // OID -> first occurrence index

	for i, entry := range entries {
		// Validate sysobjectid is not empty
		if entry.SysobjectID == "" {
			errs = append(errs, fmt.Errorf("sysobjectid_metadata[%d]: missing sysobjectid", i))
			continue
		}

		// Check for duplicate sysobjectids
		if firstIdx, exists := seenOIDs[entry.SysobjectID]; exists {
			errs = append(errs, fmt.Errorf("sysobjectid_metadata[%d]: duplicate sysobjectid %s (first occurrence at index %d)",
				i, entry.SysobjectID, firstIdx))
		} else {
			seenOIDs[entry.SysobjectID] = i
		}

		// Validate metadata fields
		for fieldName, field := range entry.Metadata {
			if !isValidMetadataField(MetadataDeviceResource, fieldName) {
				errs = append(errs, fmt.Errorf("sysobjectid_metadata[%d]: invalid resource (%s) field: %s", i, MetadataDeviceResource, fieldName))
				continue
			}

			// Validate the field must have either value or symbol(s)
			if field.Value == "" && field.Symbol.OID == "" && len(field.Symbols) == 0 {
				errs = append(errs, fmt.Errorf("sysobjectid_metadata[%d].%s: must have either value or symbol(s)", i, fieldName))
			}

			// Can't have both value and symbols
			if field.Value != "" && (field.Symbol.OID != "" || len(field.Symbols) > 0) {
				errs = append(errs, fmt.Errorf("sysobjectid_metadata[%d].%s: cannot have both value and symbol(s)", i, fieldName))
			}

			// Validate symbols if present
			for j := range field.Symbols {
				if err := validateEnrichSymbol(&field.Symbols[j], MetadataSymbol); err != nil {
					errs = append(errs, fmt.Errorf("sysobjectid_metadata[%d].%s.symbols[%d]: %s", i, fieldName, j, err))
				}
			}

			// Validate single symbol if present
			if field.Symbol.OID != "" {
				if err := validateEnrichSymbol(&field.Symbol, MetadataSymbol); err != nil {
					errs = append(errs, fmt.Errorf("sysobjectid_metadata[%d].%s.symbol: %s", i, fieldName, err))
				}
			}

			entry.Metadata[fieldName] = field
		}
	}

	return errors.Join(errs...)
}

func isValidMetadataField(resourceName, fieldName string) bool {
	fields, ok := validMetadataResources[resourceName]
	if !ok {
		return false
	}
	_, ok = fields[fieldName]
	return ok
}

func validateEnrichMetrics(metrics []MetricsConfig) error {
	var errs []error

	for i := range metrics {
		metricConfig := &metrics[i]
		if !metricConfig.IsScalar() && !metricConfig.IsColumn() {
			errs = append(errs, fmt.Errorf("either a table symbol or a scalar symbol must be provided: %#v", metricConfig))
		}
		if metricConfig.IsScalar() && metricConfig.IsColumn() {
			errs = append(errs, fmt.Errorf("table symbol and scalar symbol cannot be both provided: %#v", metricConfig))
		}
		if metricConfig.IsScalar() {
			errs = append(errs, validateEnrichSymbol(&metricConfig.Symbol, ScalarSymbol))
			for j := range metricConfig.MetricTags {
				metricTag := &metricConfig.MetricTags[j]
				errs = append(errs, validateEnrichMetricTag(metricTag))
				if metricTag.Table != "" {
					errs = append(errs, fmt.Errorf("scalar metric_tags do not support `table` lookups (tag=%q, table=%q)", metricTag.Tag, metricTag.Table))
				}
				if metricTag.Index != 0 {
					errs = append(errs, fmt.Errorf("scalar metric_tags do not support `index` lookups (tag=%q, index=%d)", metricTag.Tag, metricTag.Index))
				}
				if len(metricTag.IndexTransform) > 0 {
					errs = append(errs, fmt.Errorf("scalar metric_tags do not support `index_transform` (tag=%q)", metricTag.Tag))
				}
				if metricTag.Symbol.OID == "" {
					errs = append(errs, fmt.Errorf("scalar metric_tags require `symbol.OID` (tag=%q)", metricTag.Tag))
				}
			}
		}
		if metricConfig.IsColumn() {
			for j := range metricConfig.Symbols {
				errs = append(errs, validateEnrichSymbol(&metricConfig.Symbols[j], ColumnSymbol))
			}
			if len(metricConfig.MetricTags) == 0 {
				errs = append(errs, fmt.Errorf("column symbols doesn't have a 'metric_tags' section (%+v), all its metrics will use the same tags; "+
					"if the table has multiple rows, only one row will be submitted; "+
					"please add at least one discriminating metric tag (such as a row index) "+
					"to ensure metrics of all rows are submitted", metricConfig.Symbols))
			}
			for i := range metricConfig.MetricTags {
				metricTag := &metricConfig.MetricTags[i]
				errs = append(errs, validateEnrichMetricTag(metricTag))
			}
		}
	}

	return errors.Join(errs...)
}

func validateEnrichMetricTags(metricTags []MetricTagConfig) error {
	var errs []error
	for i := range metricTags {
		errs = append(errs, validateEnrichMetricTag(&metricTags[i]))
	}
	return errors.Join(errs...)
}

func validateEnrichSymbol(symbol *SymbolConfig, symbolContext SymbolContext) error {
	var errs []error

	if symbol.Name == "" {
		errs = append(errs, fmt.Errorf("symbol name missing: name=`%s` oid=`%s`", symbol.Name, symbol.OID))
	}
	if symbol.OID == "" {
		if symbolContext == ColumnSymbol && !symbol.ConstantValueOne {
			errs = append(errs, fmt.Errorf("symbol oid or send_as_one missing: name=`%s` oid=`%s`", symbol.Name, symbol.OID))
		} else if symbolContext != ColumnSymbol {
			errs = append(errs, fmt.Errorf("symbol oid missing: name=`%s` oid=`%s`", symbol.Name, symbol.OID))
		}
	}
	if symbol.ExtractValue != "" {
		pattern, err := regexp.Compile(symbol.ExtractValue)
		if err != nil {
			errs = append(errs, fmt.Errorf("cannot compile `extract_value` (%s): %s", symbol.ExtractValue, err.Error()))
		} else {
			symbol.ExtractValueCompiled = pattern
		}
	}
	if symbol.MatchPattern != "" {
		pattern, err := regexp.Compile(symbol.MatchPattern)
		if err != nil {
			errs = append(errs, fmt.Errorf("cannot compile `match_pattern` (%s): %s", symbol.MatchPattern, err.Error()))
		} else {
			symbol.MatchPatternCompiled = pattern
		}
	}
	errs = append(errs, validateMapping(symbol.Mapping, symbolContext))
	if symbol.Mapping.EffectiveMode() == MappingModeBitmask && symbol.Mapping.HasItems() && symbol.ScaleFactor != 0 {
		errs = append(errs, errors.New("`scale_factor` cannot be used with `mapping.mode: bitmask`"))
	}
	if symbolContext != ColumnSymbol && symbol.ConstantValueOne {
		errs = append(errs, errors.New("`constant_value_one` cannot be used outside of tables"))
	}
	if (symbolContext != ColumnSymbol && symbolContext != ScalarSymbol) && symbol.MetricType != "" {
		errs = append(errs, errors.New("`metric_type` cannot be used outside scalar/table metric symbols and metrics root"))
	}

	return errors.Join(errs...)
}

func validateEnrichMetricTag(metricTag *MetricTagConfig) error {
	var errs []error

	if (metricTag.Column.OID != "" || metricTag.Column.Name != "") && (metricTag.Symbol.OID != "" || metricTag.Symbol.Name != "") {
		errs = append(errs, fmt.Errorf("metric tag symbol and column cannot be both declared: symbol=%v, column=%v", metricTag.Symbol, metricTag.Column))
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
		errs = append(errs, fmt.Errorf("metric tag OID and symbol.OID cannot be both declared: OID=%s, symbol.OID=%s", metricTag.OID, metricTag.Symbol.OID))
	}
	if metricTag.OID != "" && metricTag.Symbol.OID == "" {
		metricTag.Symbol.OID = metricTag.OID
		metricTag.OID = ""
	}
	if isRawIndexMetricTag(*metricTag) {
		symbol := SymbolConfig(metricTag.Symbol)
		if symbol.Name == "" && metricTag.Tag == "" {
			errs = append(errs, errors.New("raw index metric tag requires `tag` or `symbol.name`"))
		}
		if symbol.ExtractValue != "" {
			pattern, err := regexp.Compile(symbol.ExtractValue)
			if err != nil {
				errs = append(errs, fmt.Errorf("cannot compile `extract_value` (%s): %s", symbol.ExtractValue, err.Error()))
			} else {
				symbol.ExtractValueCompiled = pattern
			}
		}
		if symbol.MatchPattern != "" {
			pattern, err := regexp.Compile(symbol.MatchPattern)
			if err != nil {
				errs = append(errs, fmt.Errorf("cannot compile `match_pattern` (%s): %s", symbol.MatchPattern, err.Error()))
			} else {
				symbol.MatchPatternCompiled = pattern
			}
		}
		metricTag.Symbol = SymbolConfigCompat(symbol)
	} else if metricTag.Symbol.OID != "" || metricTag.Symbol.Name != "" {
		symbol := SymbolConfig(metricTag.Symbol)
		errs = append(errs, validateEnrichSymbol(&symbol, MetricTagSymbol))
		metricTag.Symbol = SymbolConfigCompat(symbol)
	}
	if metricTag.LookupSymbol.OID != "" || metricTag.LookupSymbol.Name != "" {
		symbol := SymbolConfig(metricTag.LookupSymbol)
		errs = append(errs, validateEnrichSymbol(&symbol, MetricTagSymbol))
		metricTag.LookupSymbol = SymbolConfigCompat(symbol)
		if metricTag.Table == "" {
			errs = append(errs, errors.New("`lookup_symbol` requires `table`"))
		}
	}
	if metricTag.Match != "" {
		pattern, err := regexp.Compile(metricTag.Match)
		if err != nil {
			errs = append(errs, fmt.Errorf("cannot compile `match` (`%s`): %s", metricTag.Match, err.Error()))
		} else {
			metricTag.Pattern = pattern
		}
		if len(metricTag.Tags) == 0 {
			errs = append(errs, fmt.Errorf("`tags` mapping must be provided if `match` (`%s`) is defined", metricTag.Match))
		}
	}
	errs = append(errs, validateMapping(metricTag.Mapping, MetricTagSymbol))
	if metricTag.Mapping.HasItems() && metricTag.Tag == "" && metricTag.Symbol.Name == "" {
		errs = append(errs, fmt.Errorf("`tag` or `symbol.name` must be provided if `mapping` (`%v`) is defined", metricTag.Mapping))
	}
	for _, transform := range metricTag.IndexTransform {
		if transform.DropRight != 0 && transform.End != 0 {
			errs = append(errs, fmt.Errorf("transform rule cannot define both end and drop_right. Invalid rule: %#v", transform))
		}
		if transform.DropRight == 0 && transform.Start > transform.End && transform.End != 0 {
			errs = append(errs, fmt.Errorf("transform rule end should be greater than start. Invalid rule: %#v", transform))
		}
	}

	return errors.Join(errs...)
}

func isRawIndexMetricTag(metricTag MetricTagConfig) bool {
	if metricTag.Table != "" || metricTag.Symbol.OID != "" {
		return false
	}

	if metricTag.Index != 0 {
		return metricTag.Symbol.Format != "" ||
			metricTag.Symbol.ExtractValue != "" ||
			metricTag.Symbol.MatchPattern != "" ||
			metricTag.Mapping.HasItems()
	}

	return len(metricTag.IndexTransform) > 0 ||
		metricTag.Symbol.Format != "" ||
		metricTag.Symbol.ExtractValue != "" ||
		metricTag.Symbol.MatchPattern != "" ||
		metricTag.Mapping.HasItems()
}

func validateMapping(mapping MappingConfig, symbolContext SymbolContext) error {
	if !mapping.HasItems() {
		if mapping.Mode != "" {
			return errors.New("`mapping.mode` requires `mapping.items`")
		}
		return nil
	}

	var errs []error

	switch mapping.EffectiveMode() {
	case MappingModeExact:
	case MappingModeBitmask:
		if symbolContext != ScalarSymbol && symbolContext != ColumnSymbol {
			errs = append(errs, errors.New("`mapping.mode: bitmask` is only supported for scalar/table metric symbols"))
		}
		for key, value := range mapping.Items {
			bit, err := strconv.ParseInt(key, 10, 64)
			if err != nil || bit < 0 || (bit != 0 && bit&(bit-1) != 0) {
				errs = append(errs, fmt.Errorf("`mapping.mode: bitmask` requires keys to be 0 or a single power-of-two bit, got %q", key))
			}
			if value == "" {
				errs = append(errs, fmt.Errorf("`mapping.mode: bitmask` requires non-empty values, got empty value for key %q", key))
			}
		}
	default:
		errs = append(errs, fmt.Errorf("invalid `mapping.mode` %q", mapping.Mode))
	}

	return errors.Join(errs...)
}

func validateEnrichVirtualMetrics(metrics []MetricsConfig, vmetrics []VirtualMetricConfig) error {
	var errs []error

	metricSources := collectVirtualMetricSourceSpecs(metrics)

	seenNames := make(map[string]int)

	for i := range vmetrics {
		vm := &vmetrics[i]

		if vm.Name == "" {
			errs = append(errs, fmt.Errorf("virtual_metrics[%d]: missing name", i))
		} else {
			if firstIdx, ok := seenNames[vm.Name]; ok {
				errs = append(errs, fmt.Errorf("virtual_metrics[%d]: duplicate name %q (first occurrence at index %d)", i, vm.Name, firstIdx))
			} else {
				seenNames[vm.Name] = i
			}
			if _, ok := metricSources[vm.Name]; ok {
				errs = append(errs, fmt.Errorf("virtual_metrics[%d]: name %q conflicts with an existing metric", i, vm.Name))
			}
		}

		for j, label := range vm.GroupBy {
			if label == "" {
				errs = append(errs, fmt.Errorf("virtual_metrics[%d].group_by[%d]: label cannot be empty", i, j))
			}
		}

		for j, emitTag := range vm.EmitTags {
			if emitTag.Tag == "" {
				errs = append(errs, fmt.Errorf("virtual_metrics[%d].emit_tags[%d]: missing tag", i, j))
			}
			if emitTag.From == "" {
				errs = append(errs, fmt.Errorf("virtual_metrics[%d].emit_tags[%d]: missing from", i, j))
			}
		}

		grouped := vm.PerRow || len(vm.GroupBy) > 0

		switch {
		case len(vm.Sources) == 0 && len(vm.Alternatives) == 0:
			errs = append(errs, fmt.Errorf("virtual_metrics[%d]: must define sources or alternatives", i))
		case len(vm.Alternatives) == 0:
			errs = append(errs, validateVirtualMetricSources(fmt.Sprintf("virtual_metrics[%d].sources", i), vm.Sources, metricSources, grouped))
		default:
			for j, alt := range vm.Alternatives {
				if len(alt.Sources) == 0 {
					errs = append(errs, fmt.Errorf("virtual_metrics[%d].alternatives[%d]: must define sources", i, j))
					continue
				}
				errs = append(errs, validateVirtualMetricSources(fmt.Sprintf("virtual_metrics[%d].alternatives[%d].sources", i, j), alt.Sources, metricSources, grouped))
			}
		}
	}

	return errors.Join(errs...)
}

func validateVirtualMetricSources(path string, sources []VirtualMetricSourceConfig, metricSources map[string]map[string]virtualMetricSourceSpec, grouped bool) error {
	var errs []error

	var groupTable string
	for i, src := range sources {
		if src.Metric == "" {
			errs = append(errs, fmt.Errorf("%s[%d]: missing metric", path, i))
		}
		if grouped && src.Table == "" {
			errs = append(errs, fmt.Errorf("%s[%d]: missing table", path, i))
		}

		if src.Metric != "" {
			tables, ok := metricSources[src.Metric]
			switch {
			case !ok:
				errs = append(errs, fmt.Errorf("%s[%d]: unknown metric source %q", path, i, src.Metric))
			case src.Table == "":
				if _, ok := tables[""]; !ok {
					errs = append(errs, fmt.Errorf("%s[%d]: missing table for non-scalar source %q", path, i, src.Metric))
				}
			default:
				if _, ok := tables[src.Table]; !ok {
					errs = append(errs, fmt.Errorf("%s[%d]: unknown metric/table source %q/%q", path, i, src.Metric, src.Table))
				}
			}
		}

		if src.Dim != "" && src.Metric != "" {
			tables, ok := metricSources[src.Metric]
			if !ok {
				continue
			}

			spec, ok := tables[src.Table]
			if !ok && src.Table == "" {
				spec, ok = tables[""]
			}
			if !ok {
				continue
			}

			switch spec.dimSupport.mode {
			case virtualMetricDimUnsupported:
				errs = append(errs, fmt.Errorf("%s[%d]: dim %q requires a MultiValue source metric (%s)", path, i, src.Dim, formatVirtualMetricSourceRef(src)))
			case virtualMetricDimKnown:
				if !spec.dimSupport.dims[src.Dim] {
					errs = append(errs, fmt.Errorf("%s[%d]: dim %q is not available on %s (available: %s)", path, i, src.Dim, formatVirtualMetricSourceRef(src), strings.Join(virtualMetricSourceAvailableDims(spec), ", ")))
				}
			}
		}

		if grouped && src.Table != "" {
			if groupTable == "" {
				groupTable = src.Table
			} else if src.Table != groupTable {
				errs = append(errs, fmt.Errorf("%s[%d]: grouped virtual metrics require all sources to use the same table (saw %q and %q)", path, i, groupTable, src.Table))
			}
		}
	}

	return errors.Join(errs...)
}

func formatVirtualMetricSourceRef(src VirtualMetricSourceConfig) string {
	if src.Table == "" {
		return fmt.Sprintf("metric %q", src.Metric)
	}
	return fmt.Sprintf("metric/table %q/%q", src.Metric, src.Table)
}
