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
	TopologyScalarSymbol
	TopologyColumnSymbol
)

// ValidateEnrichProfile validates a profile and normalizes it.
func ValidateEnrichProfile(p *ProfileDefinition) error {
	normalizeMetrics(p.Metrics)
	normalizeTopology(p.Topology)
	normalizeLicensing(p.Licensing)
	normalizeBGP(p.BGP)

	errs := []error{
		validateEnrichLegacySelector(p),
		validateEnrichMetadata(p.Metadata),
		validateEnrichSysobjectIDMetadata(p.SysobjectIDMetadata),
		validateEnrichMetrics(p.Metrics),
		validateEnrichTopology(p.Topology),
		validateEnrichLicensing(p.Licensing),
		validateEnrichBGP(p.BGP),
		validateEnrichGlobalMetricTags(p.MetricTags),
		validateEnrichVirtualMetrics(p.Metrics, p.Topology, p.VirtualMetrics),
	}

	return errors.Join(errs...)
}

// normalizeMetrics converts legacy syntax to new syntax
// 1/ converts old symbol syntax to new symbol syntax
// metric.Name and metric.OID info are moved to metric.Symbol.Name and metric.Symbol.OID
func normalizeMetrics(metrics []MetricsConfig) {
	for i := range metrics {
		normalizeMetric(&metrics[i])
	}
}

func normalizeTopology(topology []TopologyConfig) {
	for i := range topology {
		normalizeMetric(&topology[i].MetricsConfig)
	}
}

func normalizeLicensing(licensing []LicensingConfig) {
	for i := range licensing {
		normalizeLicenseValue(&licensing[i].Identity.ID)
		normalizeLicenseValue(&licensing[i].Identity.Name)
		normalizeLicenseValue(&licensing[i].Identity.Feature)
		normalizeLicenseValue(&licensing[i].Identity.Component)
		normalizeLicenseValue(&licensing[i].Descriptors.Type)
		normalizeLicenseValue(&licensing[i].Descriptors.Impact)
		normalizeLicenseValue(&licensing[i].Descriptors.Perpetual)
		normalizeLicenseValue(&licensing[i].Descriptors.Unlimited)
		normalizeLicenseValue(&licensing[i].State.LicenseValueConfig)
		normalizeLicenseSignals(&licensing[i].Signals)
	}
}

func normalizeBGP(rows []BGPConfig) {
	for i := range rows {
		normalizeBGPValue(&rows[i].Identity.RoutingInstance)
		normalizeBGPValue(&rows[i].Identity.Neighbor)
		normalizeBGPValue(&rows[i].Identity.RemoteAS)
		normalizeBGPValue(&rows[i].Identity.AddressFamily.BGPValueConfig)
		normalizeBGPValue(&rows[i].Identity.SubsequentAddressFamily.BGPValueConfig)
		normalizeBGPValue(&rows[i].Descriptors.LocalAddress)
		normalizeBGPValue(&rows[i].Descriptors.LocalAS)
		normalizeBGPValue(&rows[i].Descriptors.LocalIdentifier)
		normalizeBGPValue(&rows[i].Descriptors.PeerIdentifier)
		normalizeBGPValue(&rows[i].Descriptors.PeerType)
		normalizeBGPValue(&rows[i].Descriptors.BGPVersion)
		normalizeBGPValue(&rows[i].Descriptors.Description)
		forEachBGPSignalValueConfig(&rows[i], func(_ string, value *BGPValueConfig) {
			normalizeBGPValue(value)
		})
	}
}

func normalizeBGPValue(value *BGPValueConfig) {
	if value.Symbol.Name == "" && value.Symbol.OID == "" && value.Name != "" && value.OID != "" {
		value.Symbol.Name = value.Name
		value.Symbol.OID = value.OID
		value.Name = ""
		value.OID = ""
	}
	if value.Symbol.Format == "" {
		value.Symbol.Format = value.Format
		value.Format = ""
	}
}

func normalizeLicenseSignals(signals *LicenseSignalsConfig) {
	normalizeLicenseTimerSignals(&signals.Expiry)
	normalizeLicenseTimerSignals(&signals.Authorization)
	normalizeLicenseTimerSignals(&signals.Certificate)
	normalizeLicenseTimerSignals(&signals.Grace)
	normalizeLicenseValue(&signals.Usage.Used)
	normalizeLicenseValue(&signals.Usage.Capacity)
	normalizeLicenseValue(&signals.Usage.Available)
	normalizeLicenseValue(&signals.Usage.Percent)
}

func normalizeLicenseTimerSignals(signals *LicenseTimerSignalsConfig) {
	normalizeLicenseValue(&signals.LicenseValueConfig)
	normalizeLicenseValue(&signals.Timestamp)
	normalizeLicenseValue(&signals.Remaining)
}

func normalizeLicenseValue(value *LicenseValueConfig) {
	if value.Symbol.Name == "" && value.Symbol.OID == "" && value.Name != "" && value.OID != "" {
		value.Symbol.Name = value.Name
		value.Symbol.OID = value.OID
		value.Name = ""
		value.OID = ""
	}
	if value.Symbol.Format == "" {
		value.Symbol.Format = value.Format
		value.Format = ""
	}
}

func normalizeMetric(metric *MetricsConfig) {
	if metric == nil {
		return
	}

	// converts old symbol syntax to new symbol syntax
	if metric.Symbol.Name == "" && metric.Symbol.OID == "" && metric.Name != "" && metric.OID != "" {
		metric.Symbol.Name = metric.Name
		metric.Symbol.OID = metric.OID
		metric.Name = ""
		metric.OID = ""
	}
	if metric.Symbol.MetricType == "" {
		metric.Symbol.MetricType = metric.MetricType
		metric.MetricType = ""
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
				errs = append(errs, validateConsumers(fmt.Sprintf("metadata.%s.fields.%s.consumers", resName, fieldName), field.Consumers))
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
			errs = append(errs, validateConsumers(fmt.Sprintf("sysobjectid_metadata[%d].%s.consumers", i, fieldName), field.Consumers))

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
			errs = append(errs, fmt.Errorf("topology[%d]: either a table symbol or a scalar symbol must be provided: %#v", i, metricConfig))
		}
		if metricConfig.IsScalar() && metricConfig.IsColumn() {
			errs = append(errs, fmt.Errorf("topology[%d]: table symbol and scalar symbol cannot be both provided: %#v", i, metricConfig))
		}
		if metricConfig.Options != (MetricsConfigOption{}) {
			errs = append(errs, fmt.Errorf("topology[%d]: options cannot be used in topology rows", i))
		}
		if metricConfig.IsScalar() {
			errs = append(errs, validateEnrichTopologySymbol(i, &metricConfig.Symbol, TopologyScalarSymbol))
			for j := range metricConfig.MetricTags {
				metricTag := &metricConfig.MetricTags[j]
				errs = append(errs, validateEnrichMetricTag(metricTag))
				if metricTag.Table != "" {
					errs = append(errs, fmt.Errorf("topology[%d].metric_tags[%d]: scalar metric_tags do not support `table` lookups (tag=%q, table=%q)", i, j, metricTag.Tag, metricTag.Table))
				}
				if metricTag.Index != 0 {
					errs = append(errs, fmt.Errorf("topology[%d].metric_tags[%d]: scalar metric_tags do not support `index` lookups (tag=%q, index=%d)", i, j, metricTag.Tag, metricTag.Index))
				}
				if len(metricTag.IndexTransform) > 0 {
					errs = append(errs, fmt.Errorf("topology[%d].metric_tags[%d]: scalar metric_tags do not support `index_transform` (tag=%q)", i, j, metricTag.Tag))
				}
				if metricTag.Symbol.OID == "" {
					errs = append(errs, fmt.Errorf("topology[%d].metric_tags[%d]: scalar metric_tags require `symbol.OID` (tag=%q)", i, j, metricTag.Tag))
				}
			}
		}
		if metricConfig.IsColumn() {
			for j := range metricConfig.Symbols {
				errs = append(errs, validateEnrichTopologySymbol(i, &metricConfig.Symbols[j], TopologyColumnSymbol))
			}
			for j := range metricConfig.MetricTags {
				errs = append(errs, validateEnrichMetricTag(&metricConfig.MetricTags[j]))
			}
		}
	}

	return errors.Join(errs...)
}

func validateEnrichLicensing(licensing []LicensingConfig) error {
	var errs []error
	seenSignals := make(map[licenseSignalValidationKey]string)

	for i := range licensing {
		row := &licensing[i]
		isTable := row.Table.OID != ""
		errs = append(errs, validateEnrichLicenseRowShape(i, row))
		errs = append(errs, validateEnrichLicenseValue(fmt.Sprintf("licensing[%d].identity.id", i), &row.Identity.ID, isTable))
		errs = append(errs, validateEnrichLicenseValue(fmt.Sprintf("licensing[%d].identity.name", i), &row.Identity.Name, isTable))
		errs = append(errs, validateEnrichLicenseValue(fmt.Sprintf("licensing[%d].identity.feature", i), &row.Identity.Feature, isTable))
		errs = append(errs, validateEnrichLicenseValue(fmt.Sprintf("licensing[%d].identity.component", i), &row.Identity.Component, isTable))
		errs = append(errs, validateEnrichLicenseValue(fmt.Sprintf("licensing[%d].descriptors.type", i), &row.Descriptors.Type, isTable))
		errs = append(errs, validateEnrichLicenseValue(fmt.Sprintf("licensing[%d].descriptors.impact", i), &row.Descriptors.Impact, isTable))
		errs = append(errs, validateEnrichLicenseValue(fmt.Sprintf("licensing[%d].descriptors.perpetual", i), &row.Descriptors.Perpetual, isTable))
		errs = append(errs, validateEnrichLicenseValue(fmt.Sprintf("licensing[%d].descriptors.unlimited", i), &row.Descriptors.Unlimited, isTable))
		errs = append(errs, validateEnrichLicenseState(i, &row.State, isTable))
		errs = append(errs, validateEnrichLicenseSignals(i, &row.Signals, isTable))
		errs = append(errs, validateLicenseFromReferences(i, row))
		errs = append(errs, validateLicenseSignalDuplicates(i, row, seenSignals))
		for j := range row.MetricTags {
			errs = append(errs, validateEnrichMetricTag(&row.MetricTags[j]))
			if !isTable {
				metricTag := &row.MetricTags[j]
				if metricTag.Table != "" {
					errs = append(errs, fmt.Errorf("licensing[%d].metric_tags[%d]: scalar metric_tags do not support `table` lookups (tag=%q, table=%q)", i, j, metricTag.Tag, metricTag.Table))
				}
				if metricTag.Index != 0 {
					errs = append(errs, fmt.Errorf("licensing[%d].metric_tags[%d]: scalar metric_tags do not support `index` lookups (tag=%q, index=%d)", i, j, metricTag.Tag, metricTag.Index))
				}
				if len(metricTag.IndexTransform) > 0 {
					errs = append(errs, fmt.Errorf("licensing[%d].metric_tags[%d]: scalar metric_tags do not support `index_transform` (tag=%q)", i, j, metricTag.Tag))
				}
				if metricTag.Symbol.OID == "" {
					errs = append(errs, fmt.Errorf("licensing[%d].metric_tags[%d]: scalar metric_tags require `symbol.OID` (tag=%q)", i, j, metricTag.Tag))
				}
			}
		}
	}

	return errors.Join(errs...)
}

func validateEnrichLicenseRowShape(rowIdx int, row *LicensingConfig) error {
	var errs []error

	if row.Table.Name != "" && row.Table.OID == "" {
		errs = append(errs, fmt.Errorf("licensing[%d].table: table name %q requires table OID", rowIdx, row.Table.Name))
	}
	if row.Table.OID != "" && row.Table.Name == "" {
		errs = append(errs, fmt.Errorf("licensing[%d].table: table OID %q requires table name", rowIdx, row.Table.OID))
	}
	if !licenseRowHasSignalConfigs(*row) {
		errs = append(errs, fmt.Errorf("licensing[%d]: must define state or at least one signal", rowIdx))
	}
	if row.Table.OID == "" && row.ID == "" {
		sourceOIDs := collectLicenseSignalSourceOIDs(*row)
		switch {
		case len(sourceOIDs) == 0:
			errs = append(errs, fmt.Errorf("licensing[%d]: scalar rows without a signal source OID require explicit id", rowIdx))
		case len(sourceOIDs) > 1:
			errs = append(errs, fmt.Errorf("licensing[%d]: scalar rows with multiple signal source OIDs require explicit id", rowIdx))
		}
	}

	return errors.Join(errs...)
}

func validateEnrichLicenseState(rowIdx int, state *LicenseStateConfig, isTable bool) error {
	var errs []error
	if state.Policy != "" && !IsValidLicenseStatePolicy(state.Policy) {
		errs = append(errs, fmt.Errorf("licensing[%d].state.policy: invalid policy %q", rowIdx, state.Policy))
	}
	if state.Policy != "" && !state.LicenseValueConfig.IsSet() {
		errs = append(errs, fmt.Errorf("licensing[%d].state.policy: policy requires state value source", rowIdx))
	}
	errs = append(errs, validateEnrichLicenseValueKind(fmt.Sprintf("licensing[%d].state", rowIdx), &state.LicenseValueConfig, LicenseSignalStateSeverity, isTable))
	return errors.Join(errs...)
}

func validateEnrichLicenseSignals(rowIdx int, signals *LicenseSignalsConfig, isTable bool) error {
	var errs []error
	errs = append(errs, validateEnrichLicenseTimerSignals(rowIdx, "expiry", &signals.Expiry, LicenseSignalExpiryTimestamp, LicenseSignalExpiryRemaining, isTable))
	errs = append(errs, validateEnrichLicenseTimerSignals(rowIdx, "authorization", &signals.Authorization, LicenseSignalAuthorizationTimestamp, LicenseSignalAuthorizationRemaining, isTable))
	errs = append(errs, validateEnrichLicenseTimerSignals(rowIdx, "certificate", &signals.Certificate, LicenseSignalCertificateTimestamp, LicenseSignalCertificateRemaining, isTable))
	errs = append(errs, validateEnrichLicenseTimerSignals(rowIdx, "grace", &signals.Grace, LicenseSignalGraceTimestamp, LicenseSignalGraceRemaining, isTable))
	errs = append(errs, validateEnrichLicenseValueKind(fmt.Sprintf("licensing[%d].signals.usage.used", rowIdx), &signals.Usage.Used, LicenseSignalUsageUsed, isTable))
	errs = append(errs, validateEnrichLicenseValueKind(fmt.Sprintf("licensing[%d].signals.usage.capacity", rowIdx), &signals.Usage.Capacity, LicenseSignalUsageCapacity, isTable))
	errs = append(errs, validateEnrichLicenseValueKind(fmt.Sprintf("licensing[%d].signals.usage.available", rowIdx), &signals.Usage.Available, LicenseSignalUsageAvailable, isTable))
	errs = append(errs, validateEnrichLicenseValueKind(fmt.Sprintf("licensing[%d].signals.usage.percent", rowIdx), &signals.Usage.Percent, LicenseSignalUsagePercent, isTable))
	return errors.Join(errs...)
}

func validateEnrichLicenseTimerSignals(rowIdx int, name string, signals *LicenseTimerSignalsConfig, timestampKind, remainingKind LicenseSignalKind, isTable bool) error {
	var errs []error
	basePath := fmt.Sprintf("licensing[%d].signals.%s", rowIdx, name)
	if signals.LicenseValueConfig.IsSet() && signals.Timestamp.IsSet() {
		errs = append(errs, fmt.Errorf("%s: inline timestamp and timestamp cannot both be set", basePath))
	}
	if (signals.LicenseValueConfig.IsSet() || signals.Timestamp.IsSet()) && signals.Remaining.IsSet() {
		errs = append(errs, fmt.Errorf("%s: timestamp and remaining cannot both be set", basePath))
	}
	errs = append(errs, validateEnrichLicenseValueKind(basePath, &signals.LicenseValueConfig, timestampKind, isTable))
	errs = append(errs, validateEnrichLicenseValueKind(basePath+".timestamp", &signals.Timestamp, timestampKind, isTable))
	errs = append(errs, validateEnrichLicenseValueKind(basePath+".remaining", &signals.Remaining, remainingKind, isTable))
	return errors.Join(errs...)
}

func validateEnrichLicenseValueKind(path string, value *LicenseValueConfig, defaultKind LicenseSignalKind, isTable bool) error {
	if !value.IsSet() {
		return nil
	}
	if value.Kind == "" {
		value.Kind = defaultKind
	} else if value.Kind != defaultKind {
		if !IsValidLicenseSignalKind(value.Kind) {
			return fmt.Errorf("%s.kind: invalid kind %q", path, value.Kind)
		}
		return fmt.Errorf("%s.kind: expected %q, got %q", path, defaultKind, value.Kind)
	}
	return validateEnrichLicenseValue(path, value, isTable)
}

func validateEnrichLicenseValue(path string, value *LicenseValueConfig, isTable bool) error {
	var errs []error
	if !value.IsSet() {
		return nil
	}

	if value.Kind != "" && !IsValidLicenseSignalKind(value.Kind) {
		errs = append(errs, fmt.Errorf("%s.kind: invalid kind %q", path, value.Kind))
	}
	if !licenseValueHasSource(*value) {
		errs = append(errs, fmt.Errorf("%s: must define value, from, symbol.OID, OID, index, or index_transform", path))
	}
	for i, policy := range value.Sentinel {
		if !IsValidLicenseSentinelPolicy(policy) {
			errs = append(errs, fmt.Errorf("%s.sentinel[%d]: invalid policy %q", path, i, policy))
		}
	}
	errs = append(errs, validateMapping(value.Mapping, MetadataSymbol))

	if strings.HasPrefix(value.Name, "_") {
		errs = append(errs, fmt.Errorf("%s.name: name %q cannot be underscore-prefixed", path, value.Name))
	}
	if !isTable {
		if value.Index != 0 {
			errs = append(errs, fmt.Errorf("%s.index: scalar licensing values do not support `index` lookups", path))
		}
		if len(value.IndexTransform) > 0 {
			errs = append(errs, fmt.Errorf("%s.index_transform: scalar licensing values do not support `index_transform`", path))
		}
	}
	if value.Format != "" && !isValidLicenseValueFormat(value.Format) {
		errs = append(errs, fmt.Errorf("%s.format: invalid format %q", path, value.Format))
	}
	if value.Symbol.Format != "" && !isValidLicenseValueFormat(value.Symbol.Format) {
		errs = append(errs, fmt.Errorf("%s.symbol: invalid format %q", path, value.Symbol.Format))
	}
	if value.Symbol.OID != "" || value.Symbol.Name != "" {
		errs = append(errs, validateEnrichLicenseSymbol(path, &value.Symbol))
	}

	return errors.Join(errs...)
}

func validateEnrichLicenseSymbol(path string, symbol *SymbolConfig) error {
	var errs []error

	errs = append(errs, validateEnrichSymbol(symbol, MetadataSymbol))
	if strings.HasPrefix(symbol.Name, "_") {
		errs = append(errs, fmt.Errorf("%s.symbol: name %q cannot be underscore-prefixed", path, symbol.Name))
	}
	if symbol.ChartMeta != (ChartMeta{}) {
		errs = append(errs, fmt.Errorf("%s.symbol: chart_meta cannot be used in licensing rows", path))
	}
	if symbol.MetricType != "" {
		errs = append(errs, fmt.Errorf("%s.symbol: metric_type cannot be used in licensing rows", path))
	}
	if symbol.Transform != "" {
		errs = append(errs, fmt.Errorf("%s.symbol: transform cannot be used in licensing rows", path))
	}
	if symbol.ExtractValue != "" {
		errs = append(errs, fmt.Errorf("%s.symbol: extract_value cannot be used in licensing rows", path))
	}
	if symbol.MatchPattern != "" {
		errs = append(errs, fmt.Errorf("%s.symbol: match_pattern cannot be used in licensing rows", path))
	}
	if symbol.MatchValue != "" {
		errs = append(errs, fmt.Errorf("%s.symbol: match_value cannot be used in licensing rows", path))
	}
	if symbol.ScaleFactor != 0 {
		errs = append(errs, fmt.Errorf("%s.symbol: scale_factor cannot be used in licensing rows", path))
	}
	if symbol.ConstantValueOne {
		errs = append(errs, fmt.Errorf("%s.symbol: constant_value_one cannot be used in licensing rows", path))
	}

	return errors.Join(errs...)
}

var validLicenseValueFormats = map[string]struct{}{
	"hex":              {},
	"ip_address":       {},
	"mac_address":      {},
	"snmp_dateandtime": {},
	"text_date":        {},
}

func isValidLicenseValueFormat(format string) bool {
	_, ok := validLicenseValueFormats[format]
	return ok
}

type licenseSignalValidationKey struct {
	identity string
	kind     LicenseSignalKind
}

func validateLicenseSignalDuplicates(rowIdx int, row *LicensingConfig, seen map[licenseSignalValidationKey]string) error {
	var errs []error
	identity := LicenseStructuralIdentity(*row)
	for _, sig := range collectLicenseSignalValues(*row) {
		if sig.kind == "" || !sig.value.IsSet() {
			continue
		}
		path := fmt.Sprintf("licensing[%d].%s", rowIdx, sig.path)
		key := licenseSignalValidationKey{identity: identity, kind: sig.kind}
		if prev, ok := seen[key]; ok {
			errs = append(errs, fmt.Errorf("%s: duplicate signal kind %q for structural identity %q (first seen at %s)", path, sig.kind, identity, prev))
			continue
		}
		seen[key] = path
	}
	return errors.Join(errs...)
}

func validateLicenseFromReferences(rowIdx int, row *LicensingConfig) error {
	if row.Table.OID == "" {
		return nil
	}

	var errs []error
	tableOID := TrimLicenseOID(row.Table.OID)
	for _, ref := range collectLicenseValueReferences(*row) {
		if ref.value.From == "" {
			continue
		}
		fromOID := TrimLicenseOID(ref.value.From)
		if !oidHasPrefix(fromOID, tableOID) {
			errs = append(errs, fmt.Errorf("licensing[%d].%s.from: OID %q is outside table %q", rowIdx, ref.path, ref.value.From, row.Table.OID))
		}
	}
	return errors.Join(errs...)
}

type licenseValueValidationRef struct {
	path  string
	value LicenseValueConfig
}

type licenseSignalValueValidationRef struct {
	path  string
	kind  LicenseSignalKind
	value LicenseValueConfig
}

func collectLicenseSignalValues(row LicensingConfig) []licenseSignalValueValidationRef {
	var values []licenseSignalValueValidationRef
	add := func(path string, value LicenseValueConfig) {
		if value.IsSet() {
			values = append(values, licenseSignalValueValidationRef{path: path, kind: value.Kind, value: value})
		}
	}
	add("state", row.State.LicenseValueConfig)
	collectLicenseTimerSignalValues("signals.expiry", row.Signals.Expiry, add)
	collectLicenseTimerSignalValues("signals.authorization", row.Signals.Authorization, add)
	collectLicenseTimerSignalValues("signals.certificate", row.Signals.Certificate, add)
	collectLicenseTimerSignalValues("signals.grace", row.Signals.Grace, add)
	add("signals.usage.used", row.Signals.Usage.Used)
	add("signals.usage.capacity", row.Signals.Usage.Capacity)
	add("signals.usage.available", row.Signals.Usage.Available)
	add("signals.usage.percent", row.Signals.Usage.Percent)
	return values
}

func collectLicenseTimerSignalValues(path string, cfg LicenseTimerSignalsConfig, add func(string, LicenseValueConfig)) {
	add(path, cfg.LicenseValueConfig)
	add(path+".timestamp", cfg.Timestamp)
	add(path+".remaining", cfg.Remaining)
}

func collectLicenseValueReferences(row LicensingConfig) []licenseValueValidationRef {
	var values []licenseValueValidationRef
	add := func(path string, value LicenseValueConfig) {
		if value.IsSet() {
			values = append(values, licenseValueValidationRef{path: path, value: value})
		}
	}
	add("identity.id", row.Identity.ID)
	add("identity.name", row.Identity.Name)
	add("identity.feature", row.Identity.Feature)
	add("identity.component", row.Identity.Component)
	add("descriptors.type", row.Descriptors.Type)
	add("descriptors.impact", row.Descriptors.Impact)
	add("descriptors.perpetual", row.Descriptors.Perpetual)
	add("descriptors.unlimited", row.Descriptors.Unlimited)
	add("state", row.State.LicenseValueConfig)
	collectLicenseTimerSignalValues("signals.expiry", row.Signals.Expiry, add)
	collectLicenseTimerSignalValues("signals.authorization", row.Signals.Authorization, add)
	collectLicenseTimerSignalValues("signals.certificate", row.Signals.Certificate, add)
	collectLicenseTimerSignalValues("signals.grace", row.Signals.Grace, add)
	add("signals.usage.used", row.Signals.Usage.Used)
	add("signals.usage.capacity", row.Signals.Usage.Capacity)
	add("signals.usage.available", row.Signals.Usage.Available)
	add("signals.usage.percent", row.Signals.Usage.Percent)
	return values
}

func licenseRowHasSignalConfigs(row LicensingConfig) bool {
	if row.State.LicenseValueConfig.IsSet() {
		return true
	}
	for _, sig := range collectLicenseSignalValues(row) {
		if sig.value.IsSet() {
			return true
		}
	}
	return false
}

func collectLicenseSignalSourceOIDs(row LicensingConfig) map[string]struct{} {
	oids := make(map[string]struct{})
	for _, sig := range collectLicenseSignalValues(row) {
		if oid := LicenseValueSourceOID(sig.value); oid != "" {
			oids[TrimLicenseOID(oid)] = struct{}{}
		}
	}
	return oids
}

func licenseValueHasSource(value LicenseValueConfig) bool {
	return value.Value != "" ||
		value.From != "" ||
		value.Symbol.OID != "" ||
		value.OID != "" ||
		value.Index != 0 ||
		len(value.IndexTransform) > 0
}

func oidHasPrefix(oid, prefix string) bool {
	return oid == prefix || strings.HasPrefix(oid, prefix+".")
}

func validateEnrichBGP(rows []BGPConfig) error {
	var errs []error
	seenSignals := make(map[bgpSignalValidationKey]string)

	for i := range rows {
		row := &rows[i]
		isTable := row.Table.OID != ""

		errs = append(errs, validateEnrichBGPRowShape(i, row))
		errs = append(errs, validateEnrichBGPValue(fmt.Sprintf("bgp[%d].identity.routing_instance", i), &row.Identity.RoutingInstance, isTable))
		errs = append(errs, validateEnrichBGPValue(fmt.Sprintf("bgp[%d].identity.neighbor", i), &row.Identity.Neighbor, isTable))
		errs = append(errs, validateEnrichBGPValue(fmt.Sprintf("bgp[%d].identity.remote_as", i), &row.Identity.RemoteAS, isTable))
		errs = append(errs, validateEnrichBGPAddressFamilyValue(fmt.Sprintf("bgp[%d].identity.address_family", i), &row.Identity.AddressFamily, isTable))
		errs = append(errs, validateEnrichBGPSubsequentAddressFamilyValue(fmt.Sprintf("bgp[%d].identity.subsequent_address_family", i), &row.Identity.SubsequentAddressFamily, isTable))
		errs = append(errs, validateEnrichBGPValue(fmt.Sprintf("bgp[%d].descriptors.local_address", i), &row.Descriptors.LocalAddress, isTable))
		errs = append(errs, validateEnrichBGPValue(fmt.Sprintf("bgp[%d].descriptors.local_as", i), &row.Descriptors.LocalAS, isTable))
		errs = append(errs, validateEnrichBGPValue(fmt.Sprintf("bgp[%d].descriptors.local_identifier", i), &row.Descriptors.LocalIdentifier, isTable))
		errs = append(errs, validateEnrichBGPValue(fmt.Sprintf("bgp[%d].descriptors.peer_identifier", i), &row.Descriptors.PeerIdentifier, isTable))
		errs = append(errs, validateEnrichBGPValue(fmt.Sprintf("bgp[%d].descriptors.peer_type", i), &row.Descriptors.PeerType, isTable))
		errs = append(errs, validateEnrichBGPValue(fmt.Sprintf("bgp[%d].descriptors.bgp_version", i), &row.Descriptors.BGPVersion, isTable))
		errs = append(errs, validateEnrichBGPValue(fmt.Sprintf("bgp[%d].descriptors.description", i), &row.Descriptors.Description, isTable))
		errs = append(errs, validateEnrichBGPSignalValues(i, row, isTable))
		errs = append(errs, validateBGPGroupKindCompatibility(i, row))
		errs = append(errs, validateBGPFromReferences(i, row))
		errs = append(errs, validateBGPSignalDuplicates(i, row, seenSignals))

		for j := range row.MetricTags {
			errs = append(errs, validateEnrichMetricTag(&row.MetricTags[j]))
			if !isTable {
				metricTag := &row.MetricTags[j]
				if metricTag.Table != "" {
					errs = append(errs, fmt.Errorf("bgp[%d].metric_tags[%d]: scalar metric_tags do not support `table` lookups (tag=%q, table=%q)", i, j, metricTag.Tag, metricTag.Table))
				}
				if metricTag.Index != 0 {
					errs = append(errs, fmt.Errorf("bgp[%d].metric_tags[%d]: scalar metric_tags do not support `index` lookups (tag=%q, index=%d)", i, j, metricTag.Tag, metricTag.Index))
				}
				if len(metricTag.IndexTransform) > 0 {
					errs = append(errs, fmt.Errorf("bgp[%d].metric_tags[%d]: scalar metric_tags do not support `index_transform` (tag=%q)", i, j, metricTag.Tag))
				}
				if metricTag.Symbol.OID == "" {
					errs = append(errs, fmt.Errorf("bgp[%d].metric_tags[%d]: scalar metric_tags require `symbol.OID` (tag=%q)", i, j, metricTag.Tag))
				}
			}
		}
	}

	errs = append(errs, validateBGPTableReferences(rows))

	return errors.Join(errs...)
}

func forEachBGPSignalValueConfig(row *BGPConfig, add func(path string, value *BGPValueConfig)) {
	addState := func(path string, value *BGPStateConfig) {
		add(path, &value.BGPValueConfig)
	}
	addDirectional := func(prefix string, value *BGPDirectionalConfig) {
		add(prefix+".received", &value.Received)
		add(prefix+".sent", &value.Sent)
	}
	addTimerPair := func(prefix string, value *BGPTimerPairConfig) {
		add(prefix+".connect_retry", &value.ConnectRetry)
		add(prefix+".hold_time", &value.HoldTime)
		add(prefix+".keepalive_time", &value.KeepaliveTime)
		add(prefix+".min_as_origination_interval", &value.MinASOriginationInterval)
		add(prefix+".min_route_advertisement_interval", &value.MinRouteAdvertisementInterval)
	}
	addNotification := func(prefix string, value *BGPLastNotificationConfig) {
		add(prefix+".code", &value.Code)
		add(prefix+".subcode", &value.Subcode)
		add(prefix+".reason", &value.Reason)
	}
	addRoutes := func(prefix string, value *BGPRouteCountersConfig) {
		add(prefix+".received", &value.Received)
		add(prefix+".accepted", &value.Accepted)
		add(prefix+".rejected", &value.Rejected)
		add(prefix+".active", &value.Active)
		add(prefix+".advertised", &value.Advertised)
		add(prefix+".suppressed", &value.Suppressed)
		add(prefix+".withdrawn", &value.Withdrawn)
	}

	add("admin.enabled", &row.Admin.Enabled)
	addState("state", &row.State)
	addState("previous_state", &row.Previous)
	add("connection.established_uptime", &row.Connection.EstablishedUptime)
	add("connection.last_received_update_age", &row.Connection.LastReceivedUpdateAge)
	addDirectional("traffic.messages", &row.Traffic.Messages)
	addDirectional("traffic.updates", &row.Traffic.Updates)
	addDirectional("traffic.notifications", &row.Traffic.Notifications)
	addDirectional("traffic.route_refreshes", &row.Traffic.RouteRefreshes)
	addDirectional("traffic.opens", &row.Traffic.Opens)
	addDirectional("traffic.keepalives", &row.Traffic.Keepalives)
	add("transitions.established", &row.Transitions.Established)
	add("transitions.down", &row.Transitions.Down)
	add("transitions.up", &row.Transitions.Up)
	add("transitions.flaps", &row.Transitions.Flaps)
	addTimerPair("timers.negotiated", &row.Timers.Negotiated)
	addTimerPair("timers.configured", &row.Timers.Configured)
	add("last_error.code", &row.LastError.Code)
	add("last_error.subcode", &row.LastError.Subcode)
	addNotification("last_notifications.received", &row.LastNotify.Received)
	addNotification("last_notifications.sent", &row.LastNotify.Sent)
	add("reasons.last_down", &row.Reasons.LastDown)
	add("reasons.unavailability", &row.Reasons.Unavailability)
	add("graceful_restart.state", &row.Restart.State)
	addRoutes("routes.current", &row.Routes.Current)
	addRoutes("routes.total", &row.Routes.Total)
	add("route_limits.limit", &row.RouteLimits.Limit)
	add("route_limits.threshold", &row.RouteLimits.Threshold)
	add("route_limits.clear_threshold", &row.RouteLimits.ClearThreshold)
	add("device_counts.peers", &row.Device.Peers)
	add("device_counts.ibgp_peers", &row.Device.InternalPeers)
	add("device_counts.ebgp_peers", &row.Device.ExternalPeers)
	add("device_counts.states.idle", &row.Device.States.Idle)
	add("device_counts.states.connect", &row.Device.States.Connect)
	add("device_counts.states.active", &row.Device.States.Active)
	add("device_counts.states.opensent", &row.Device.States.OpenSent)
	add("device_counts.states.openconfirm", &row.Device.States.OpenConfirm)
	add("device_counts.states.established", &row.Device.States.Established)
}

func validateEnrichBGPRowShape(rowIdx int, row *BGPConfig) error {
	var errs []error

	if row.Kind == "" {
		errs = append(errs, fmt.Errorf("bgp[%d].kind: missing kind", rowIdx))
	} else if !IsValidBGPRowKind(row.Kind) {
		errs = append(errs, fmt.Errorf("bgp[%d].kind: invalid kind %q", rowIdx, row.Kind))
	}
	if row.Table.Name != "" && row.Table.OID == "" {
		errs = append(errs, fmt.Errorf("bgp[%d].table: table name %q requires table OID", rowIdx, row.Table.Name))
	}
	if row.Table.OID != "" && row.Table.Name == "" {
		errs = append(errs, fmt.Errorf("bgp[%d].table: table OID %q requires table name", rowIdx, row.Table.OID))
	}
	if !bgpRowHasSignalConfigs(*row) {
		errs = append(errs, fmt.Errorf("bgp[%d]: must define at least one typed BGP field", rowIdx))
	}

	switch row.Kind {
	case BGPRowKindPeer:
		if !row.Identity.Neighbor.IsSet() {
			errs = append(errs, fmt.Errorf("bgp[%d].identity.neighbor: peer rows require neighbor identity", rowIdx))
		}
		if !row.Identity.RemoteAS.IsSet() {
			errs = append(errs, fmt.Errorf("bgp[%d].identity.remote_as: peer rows require remote_as identity", rowIdx))
		}
	case BGPRowKindPeerFamily:
		if !row.Identity.Neighbor.IsSet() {
			errs = append(errs, fmt.Errorf("bgp[%d].identity.neighbor: peer_family rows require neighbor identity", rowIdx))
		}
		if !row.Identity.RemoteAS.IsSet() {
			errs = append(errs, fmt.Errorf("bgp[%d].identity.remote_as: peer_family rows require remote_as identity", rowIdx))
		}
		if !row.Identity.AddressFamily.IsSet() {
			errs = append(errs, fmt.Errorf("bgp[%d].identity.address_family: peer_family rows require address_family identity", rowIdx))
		}
		if !row.Identity.SubsequentAddressFamily.IsSet() {
			errs = append(errs, fmt.Errorf("bgp[%d].identity.subsequent_address_family: peer_family rows require subsequent_address_family identity", rowIdx))
		}
	}

	if row.Table.OID == "" && row.ID == "" {
		sourceOIDs := collectBGPSignalSourceOIDs(*row)
		switch {
		case len(sourceOIDs) == 0:
			errs = append(errs, fmt.Errorf("bgp[%d]: scalar rows without a typed-field source OID require explicit id", rowIdx))
		case len(sourceOIDs) > 1:
			errs = append(errs, fmt.Errorf("bgp[%d]: scalar rows with multiple typed-field source OIDs require explicit id", rowIdx))
		}
	}

	return errors.Join(errs...)
}

func validateBGPGroupKindCompatibility(rowIdx int, row *BGPConfig) error {
	var errs []error
	ForEachBGPSignalValue(*row, func(path string, _ BGPValueConfig) {
		switch {
		case row.Kind == BGPRowKindDevice && !strings.HasPrefix(path, "device_counts."):
			errs = append(errs, fmt.Errorf("bgp[%d].%s: device rows only support device_counts fields", rowIdx, path))
		case strings.HasPrefix(path, "device_counts.") && row.Kind != BGPRowKindDevice:
			errs = append(errs, fmt.Errorf("bgp[%d].%s: device_counts fields require kind=device", rowIdx, path))
		case (strings.HasPrefix(path, "routes.") || strings.HasPrefix(path, "route_limits.")) && row.Kind != BGPRowKindPeerFamily:
			errs = append(errs, fmt.Errorf("bgp[%d].%s: route fields require kind=peer_family", rowIdx, path))
		}
	})
	return errors.Join(errs...)
}

func validateEnrichBGPState(path string, state *BGPStateConfig, isTable bool) error {
	var errs []error
	if !state.BGPValueConfig.IsSet() {
		if state.Partial {
			errs = append(errs, fmt.Errorf("%s.partial: partial requires state value source", path))
		}
		return errors.Join(errs...)
	}

	errs = append(errs, validateEnrichBGPValue(path, &state.BGPValueConfig, isTable))
	errs = append(errs, validateBGPPeerStateMapping(path, state))
	return errors.Join(errs...)
}

func validateEnrichBGPSignalValues(rowIdx int, row *BGPConfig, isTable bool) error {
	var errs []error
	errs = append(errs, validateEnrichBGPState(fmt.Sprintf("bgp[%d].state", rowIdx), &row.State, isTable))
	errs = append(errs, validateEnrichBGPState(fmt.Sprintf("bgp[%d].previous_state", rowIdx), &row.Previous, isTable))
	forEachBGPSignalValueConfig(row, func(path string, value *BGPValueConfig) {
		if path == "state" || path == "previous_state" {
			return
		}
		errs = append(errs, validateEnrichBGPValue(fmt.Sprintf("bgp[%d].%s", rowIdx, path), value, isTable))
	})
	return errors.Join(errs...)
}

func validateEnrichBGPAddressFamilyValue(path string, value *BGPAddressFamilyValueConfig, isTable bool) error {
	var errs []error
	errs = append(errs, validateEnrichBGPValue(path, &value.BGPValueConfig, isTable))
	if !value.AllowPrivate {
		errs = append(errs, validateBGPAddressFamilyValues(path, value.BGPValueConfig))
	}
	return errors.Join(errs...)
}

func validateEnrichBGPSubsequentAddressFamilyValue(path string, value *BGPSubsequentAddressFamilyValueConfig, isTable bool) error {
	var errs []error
	errs = append(errs, validateEnrichBGPValue(path, &value.BGPValueConfig, isTable))
	if !value.AllowPrivate {
		errs = append(errs, validateBGPSubsequentAddressFamilyValues(path, value.BGPValueConfig))
	}
	return errors.Join(errs...)
}

func validateEnrichBGPValue(path string, value *BGPValueConfig, isTable bool) error {
	var errs []error
	if !value.IsSet() {
		return nil
	}

	if !bgpValueHasSource(*value) {
		errs = append(errs, fmt.Errorf("%s: must define value, from, symbol.OID, OID, index, index_from_end, or index_transform", path))
	}
	errs = append(errs, validateMapping(value.Mapping, MetadataSymbol))
	errs = append(errs, validateBGPRowIndexSelector(path, value))

	if strings.HasPrefix(value.Name, "_") {
		errs = append(errs, fmt.Errorf("%s.name: name %q cannot be underscore-prefixed", path, value.Name))
	}
	if !isTable {
		if value.Table != "" {
			errs = append(errs, fmt.Errorf("%s.table: scalar BGP values do not support `table` lookups", path))
		}
		if value.Index != 0 {
			errs = append(errs, fmt.Errorf("%s.index: scalar BGP values do not support `index` lookups", path))
		}
		if value.IndexFromEnd != 0 {
			errs = append(errs, fmt.Errorf("%s.index_from_end: scalar BGP values do not support `index_from_end` lookups", path))
		}
		if len(value.IndexTransform) > 0 {
			errs = append(errs, fmt.Errorf("%s.index_transform: scalar BGP values do not support `index_transform`", path))
		}
	}
	if value.Table != "" && bgpValueSymbolForValidation(*value).OID == "" {
		errs = append(errs, fmt.Errorf("%s.table: table lookups require symbol.OID, OID, or from", path))
	}
	if value.LookupSymbol.OID != "" || value.LookupSymbol.Name != "" {
		if value.Table == "" {
			errs = append(errs, fmt.Errorf("%s.lookup_symbol: lookup_symbol requires table", path))
		}
		lookupSymbol := SymbolConfig(value.LookupSymbol)
		errs = append(errs, validateEnrichBGPSymbol(path+".lookup_symbol", &lookupSymbol))
		value.LookupSymbol = SymbolConfigCompat(lookupSymbol)
	}
	if value.Symbol.OID != "" || value.Symbol.Name != "" {
		errs = append(errs, validateEnrichBGPSymbol(path, &value.Symbol))
	}

	return errors.Join(errs...)
}

func validateBGPRowIndexSelector(path string, value *BGPValueConfig) error {
	var selectors []string
	if value.Index != 0 {
		selectors = append(selectors, "index")
	}
	if value.IndexFromEnd != 0 {
		selectors = append(selectors, "index_from_end")
	}
	if len(value.IndexTransform) > 0 {
		selectors = append(selectors, "index_transform")
	}
	if len(selectors) <= 1 {
		return nil
	}
	return fmt.Errorf("%s: index, index_from_end, and index_transform are mutually exclusive (set: %s)", path, strings.Join(selectors, ", "))
}

func validateEnrichBGPSymbol(path string, symbol *SymbolConfig) error {
	var errs []error

	errs = append(errs, validateEnrichSymbol(symbol, MetadataSymbol))
	if strings.HasPrefix(symbol.Name, "_") {
		errs = append(errs, fmt.Errorf("%s.symbol: name %q cannot be underscore-prefixed", path, symbol.Name))
	}
	if symbol.ChartMeta != (ChartMeta{}) {
		errs = append(errs, fmt.Errorf("%s.symbol: chart_meta cannot be used in BGP rows", path))
	}
	if symbol.MetricType != "" {
		errs = append(errs, fmt.Errorf("%s.symbol: metric_type cannot be used in BGP rows", path))
	}
	if symbol.Transform != "" {
		errs = append(errs, fmt.Errorf("%s.symbol: transform cannot be used in BGP rows", path))
	}
	if symbol.ScaleFactor != 0 {
		errs = append(errs, fmt.Errorf("%s.symbol: scale_factor cannot be used in BGP rows", path))
	}
	if symbol.ConstantValueOne {
		errs = append(errs, fmt.Errorf("%s.symbol: constant_value_one cannot be used in BGP rows", path))
	}

	return errors.Join(errs...)
}

func validateBGPPeerStateMapping(path string, state *BGPStateConfig) error {
	mapping := effectiveBGPValueMapping(state.BGPValueConfig)
	if !mapping.HasItems() {
		if state.Partial {
			return fmt.Errorf("%s.mapping: partial state mapping requires at least one RFC 4271 state", path)
		}
		return fmt.Errorf("%s.mapping: state mapping must cover all six RFC 4271 states", path)
	}

	var errs []error
	covered := make(map[BGPPeerState]bool)
	for raw, mapped := range mapping.Items {
		peerState := BGPPeerState(mapped)
		if !IsValidBGPPeerState(peerState) {
			errs = append(errs, fmt.Errorf("%s.mapping.items[%s]: invalid BGP peer state %q", path, raw, mapped))
			continue
		}
		covered[peerState] = true
	}
	for i, partial := range state.PartialStates {
		if !IsValidBGPPeerState(partial) {
			errs = append(errs, fmt.Errorf("%s.partial_states[%d]: invalid BGP peer state %q", path, i, partial))
		}
	}
	if state.Partial {
		return errors.Join(errs...)
	}
	for _, required := range requiredBGPPeerStates {
		if !covered[required] {
			errs = append(errs, fmt.Errorf("%s.mapping: missing RFC 4271 state %q", path, required))
		}
	}
	return errors.Join(errs...)
}

func validateBGPAddressFamilyValues(path string, value BGPValueConfig) error {
	return validateBGPEnumValues(path, value, func(v string) bool {
		return IsValidBGPAddressFamily(BGPAddressFamily(v))
	}, "address_family")
}

func validateBGPSubsequentAddressFamilyValues(path string, value BGPValueConfig) error {
	return validateBGPEnumValues(path, value, func(v string) bool {
		return IsValidBGPSubsequentAddressFamily(BGPSubsequentAddressFamily(v))
	}, "subsequent_address_family")
}

func validateBGPEnumValues(path string, value BGPValueConfig, valid func(string) bool, label string) error {
	var errs []error
	if value.Value != "" && !valid(value.Value) {
		errs = append(errs, fmt.Errorf("%s.value: invalid BGP %s %q", path, label, value.Value))
	}
	for raw, mapped := range effectiveBGPValueMapping(value).Items {
		if !valid(mapped) {
			errs = append(errs, fmt.Errorf("%s.mapping.items[%s]: invalid BGP %s %q", path, raw, label, mapped))
		}
	}
	return errors.Join(errs...)
}

func effectiveBGPValueMapping(value BGPValueConfig) MappingConfig {
	if value.Mapping.HasItems() || value.Mapping.Mode != "" {
		return value.Mapping
	}
	return value.Symbol.Mapping
}

type bgpSignalValidationKey struct {
	identity string
	path     string
}

func validateBGPSignalDuplicates(rowIdx int, row *BGPConfig, seen map[bgpSignalValidationKey]string) error {
	var errs []error
	identity := BGPStructuralIdentity(*row)
	ForEachBGPSignalValue(*row, func(path string, _ BGPValueConfig) {
		fullPath := fmt.Sprintf("bgp[%d].%s", rowIdx, path)
		key := bgpSignalValidationKey{identity: identity, path: path}
		if prev, ok := seen[key]; ok {
			errs = append(errs, fmt.Errorf("%s: duplicate BGP field for structural identity %q (first seen at %s)", fullPath, identity, prev))
			return
		}
		seen[key] = fullPath
	})
	return errors.Join(errs...)
}

func validateBGPFromReferences(rowIdx int, row *BGPConfig) error {
	if row.Table.OID == "" {
		return nil
	}

	var errs []error
	tableOID := TrimBGPOID(row.Table.OID)
	for _, ref := range collectBGPValueReferences(*row) {
		if ref.value.From == "" {
			continue
		}
		if ref.value.Table != "" {
			continue
		}
		fromOID := TrimBGPOID(ref.value.From)
		if !oidHasPrefix(fromOID, tableOID) {
			errs = append(errs, fmt.Errorf("bgp[%d].%s.from: OID %q is outside table %q", rowIdx, ref.path, ref.value.From, row.Table.OID))
		}
	}
	return errors.Join(errs...)
}

func validateBGPTableReferences(rows []BGPConfig) error {
	tableNameToOID := make(map[string]string)
	for i := range rows {
		row := &rows[i]
		if row.Table.Name != "" && row.Table.OID != "" {
			tableNameToOID[row.Table.Name] = TrimBGPOID(row.Table.OID)
		}
	}

	var errs []error
	for rowIdx := range rows {
		row := &rows[rowIdx]
		for _, ref := range collectBGPValueReferences(*row) {
			if ref.value.Table == "" {
				continue
			}
			refTableOID, ok := tableNameToOID[ref.value.Table]
			if !ok {
				continue
			}
			sourceOID := TrimBGPOID(BGPValueSourceOID(ref.value))
			if sourceOID != "" && !oidHasPrefix(sourceOID, refTableOID) {
				errs = append(errs, fmt.Errorf("bgp[%d].%s.table: referenced table %q uses OID %q; source OID %q is outside referenced table", rowIdx, ref.path, ref.value.Table, refTableOID, sourceOID))
			}
			lookupOID := TrimBGPOID(SymbolConfig(ref.value.LookupSymbol).OID)
			if lookupOID != "" && !oidHasPrefix(lookupOID, refTableOID) {
				errs = append(errs, fmt.Errorf("bgp[%d].%s.lookup_symbol: referenced table %q uses OID %q; lookup OID %q is outside referenced table", rowIdx, ref.path, ref.value.Table, refTableOID, lookupOID))
			}
		}
	}
	return errors.Join(errs...)
}

type bgpValueValidationRef struct {
	path  string
	value BGPValueConfig
}

func collectBGPValueReferences(row BGPConfig) []bgpValueValidationRef {
	var values []bgpValueValidationRef
	add := func(path string, value BGPValueConfig) {
		if value.IsSet() {
			values = append(values, bgpValueValidationRef{path: path, value: value})
		}
	}
	add("identity.routing_instance", row.Identity.RoutingInstance)
	add("identity.neighbor", row.Identity.Neighbor)
	add("identity.remote_as", row.Identity.RemoteAS)
	add("identity.address_family", row.Identity.AddressFamily.BGPValueConfig)
	add("identity.subsequent_address_family", row.Identity.SubsequentAddressFamily.BGPValueConfig)
	add("descriptors.local_address", row.Descriptors.LocalAddress)
	add("descriptors.local_as", row.Descriptors.LocalAS)
	add("descriptors.local_identifier", row.Descriptors.LocalIdentifier)
	add("descriptors.peer_identifier", row.Descriptors.PeerIdentifier)
	add("descriptors.peer_type", row.Descriptors.PeerType)
	add("descriptors.bgp_version", row.Descriptors.BGPVersion)
	add("descriptors.description", row.Descriptors.Description)
	ForEachBGPSignalValue(row, add)
	return values
}

func bgpRowHasSignalConfigs(row BGPConfig) bool {
	hasSignals := false
	ForEachBGPSignalValue(row, func(_ string, _ BGPValueConfig) {
		hasSignals = true
	})
	return hasSignals
}

func collectBGPSignalSourceOIDs(row BGPConfig) map[string]struct{} {
	oids := make(map[string]struct{})
	ForEachBGPSignalValue(row, func(_ string, value BGPValueConfig) {
		if oid := BGPValueSourceOID(value); oid != "" {
			oids[TrimBGPOID(oid)] = struct{}{}
		}
	})
	return oids
}

func bgpValueHasSource(value BGPValueConfig) bool {
	return value.Value != "" ||
		value.From != "" ||
		value.Symbol.OID != "" ||
		value.OID != "" ||
		value.Index != 0 ||
		value.IndexFromEnd != 0 ||
		len(value.IndexTransform) > 0
}

func bgpValueSymbolForValidation(value BGPValueConfig) SymbolConfig {
	sym := value.Symbol
	if sym.OID == "" {
		switch {
		case value.From != "":
			sym.OID = value.From
		case value.OID != "":
			sym.OID = value.OID
		}
	}
	return sym
}

func validateEnrichTopologySymbol(topologyIdx int, symbol *SymbolConfig, symbolContext SymbolContext) error {
	var errs []error

	errs = append(errs, validateEnrichSymbol(symbol, symbolContext))
	if strings.HasPrefix(symbol.Name, "_") {
		errs = append(errs, fmt.Errorf("topology[%d]: symbol name %q cannot be underscore-prefixed", topologyIdx, symbol.Name))
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

	return errors.Join(errs...)
}

func validateEnrichGlobalMetricTags(metricTags []GlobalMetricTagConfig) error {
	var errs []error
	for i := range metricTags {
		errs = append(errs, validateEnrichMetricTag(&metricTags[i].MetricTagConfig))
		errs = append(errs, validateConsumers(fmt.Sprintf("metric_tags[%d].consumers", i), metricTags[i].Consumers))
	}
	return errors.Join(errs...)
}

func validateConsumers(path string, consumers ConsumerSet) error {
	var errs []error
	seen := make(map[ProfileConsumer]int)
	for i, consumer := range consumers {
		switch consumer {
		case ConsumerMetrics, ConsumerTopology, ConsumerLicensing, ConsumerBGP:
		default:
			errs = append(errs, fmt.Errorf("%s[%d]: invalid consumer %q", path, i, consumer))
			continue
		}
		if firstIdx, ok := seen[consumer]; ok {
			errs = append(errs, fmt.Errorf("%s[%d]: duplicate consumer %q (first occurrence at index %d)", path, i, consumer, firstIdx))
			continue
		}
		seen[consumer] = i
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

func validateEnrichVirtualMetrics(metrics []MetricsConfig, topology []TopologyConfig, vmetrics []VirtualMetricConfig) error {
	var errs []error

	metricSources := collectVirtualMetricSourceSpecs(metrics)
	topologySources := collectTopologyMetricSourceNames(topology)

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
			errs = append(errs, validateVirtualMetricSourcesNotTopology(fmt.Sprintf("virtual_metrics[%d].sources", i), vm.Sources, topologySources))
			errs = append(errs, validateVirtualMetricSources(fmt.Sprintf("virtual_metrics[%d].sources", i), vm.Sources, metricSources, grouped))
		default:
			for j, alt := range vm.Alternatives {
				if len(alt.Sources) == 0 {
					errs = append(errs, fmt.Errorf("virtual_metrics[%d].alternatives[%d]: must define sources", i, j))
					continue
				}
				errs = append(errs, validateVirtualMetricSourcesNotTopology(fmt.Sprintf("virtual_metrics[%d].alternatives[%d].sources", i, j), alt.Sources, topologySources))
				errs = append(errs, validateVirtualMetricSources(fmt.Sprintf("virtual_metrics[%d].alternatives[%d].sources", i, j), alt.Sources, metricSources, grouped))
			}
		}
	}

	return errors.Join(errs...)
}

func collectTopologyMetricSourceNames(topology []TopologyConfig) map[string]struct{} {
	names := make(map[string]struct{})
	for _, topo := range topology {
		metric := &topo.MetricsConfig
		switch {
		case metric.IsScalar():
			names[metric.Symbol.Name] = struct{}{}
		case metric.IsColumn():
			for _, sym := range metric.Symbols {
				names[sym.Name] = struct{}{}
			}
		}
	}
	return names
}

func validateVirtualMetricSourcesNotTopology(path string, sources []VirtualMetricSourceConfig, topologySources map[string]struct{}) error {
	var errs []error
	for i, src := range sources {
		if _, ok := topologySources[src.Metric]; ok {
			errs = append(errs, fmt.Errorf("%s[%d]: topology metric source %q cannot be used by virtual_metrics", path, i, src.Metric))
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
