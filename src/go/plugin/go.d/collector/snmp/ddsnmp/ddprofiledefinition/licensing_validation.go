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

func normalizeLicensing(rows []LicensingConfig) {
	for i := range rows {
		transformLicenseValues(&rows[i], func(_ string, value LicenseValueConfig) LicenseValueConfig {
			normalizeLicenseValue(&value)
			return value
		})
	}
}

func normalizeLicenseValue(value *LicenseValueConfig) {
	if value.From == "" && value.Symbol.Name == "" && value.Symbol.OID == "" && value.Name != "" && value.OID != "" {
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

func validateEnrichLicensing(licensing []LicensingConfig) error {
	var errs []error
	seenSignals := make(map[licenseSignalValidationKey]string)

	for i := range licensing {
		row := &licensing[i]
		isTable := row.Table.OID != ""
		errs = append(errs, validateEnrichLicenseRowShape(i, row))
		errs = append(
			errs,
			validateEnrichLicenseValue(fmt.Sprintf("licensing[%d].identity.id", i), &row.Identity.ID, isTable),
		)
		errs = append(
			errs,
			validateEnrichLicenseValue(fmt.Sprintf("licensing[%d].identity.name", i), &row.Identity.Name, isTable),
		)
		errs = append(
			errs,
			validateEnrichLicenseValue(
				fmt.Sprintf("licensing[%d].identity.feature", i),
				&row.Identity.Feature,
				isTable,
			),
		)
		errs = append(
			errs,
			validateEnrichLicenseValue(
				fmt.Sprintf("licensing[%d].identity.component", i),
				&row.Identity.Component,
				isTable,
			),
		)
		errs = append(
			errs,
			validateEnrichLicenseValue(
				fmt.Sprintf("licensing[%d].descriptors.type", i),
				&row.Descriptors.Type,
				isTable,
			),
		)
		errs = append(
			errs,
			validateEnrichLicenseValue(
				fmt.Sprintf("licensing[%d].descriptors.impact", i),
				&row.Descriptors.Impact,
				isTable,
			),
		)
		errs = append(
			errs,
			validateEnrichLicenseValue(
				fmt.Sprintf("licensing[%d].descriptors.perpetual", i),
				&row.Descriptors.Perpetual,
				isTable,
			),
		)
		errs = append(
			errs,
			validateEnrichLicenseValue(
				fmt.Sprintf("licensing[%d].descriptors.unlimited", i),
				&row.Descriptors.Unlimited,
				isTable,
			),
		)
		errs = append(errs, validateEnrichLicenseState(i, &row.State, isTable))
		errs = append(errs, validateEnrichLicenseSignals(i, &row.Signals, isTable))
		errs = append(errs, validateLicenseSourceReferences(i, row))
		errs = append(errs, validateLicenseSignalDuplicates(i, row, seenSignals))
		errs = append(errs, validateEnrichMetricTags(fmt.Sprintf("licensing[%d]", i), row.MetricTags, !isTable))
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
			errs = append(
				errs,
				fmt.Errorf("licensing[%d]: scalar rows without a signal source OID require explicit id", rowIdx),
			)
		case len(sourceOIDs) > 1:
			errs = append(
				errs,
				fmt.Errorf("licensing[%d]: scalar rows with multiple signal source OIDs require explicit id", rowIdx),
			)
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
	errs = append(
		errs,
		validateEnrichLicenseValueKind(
			fmt.Sprintf("licensing[%d].state", rowIdx),
			&state.LicenseValueConfig,
			LicenseSignalStateSeverity,
			isTable,
		),
	)
	return errors.Join(errs...)
}

func validateEnrichLicenseSignals(rowIdx int, signals *LicenseSignalsConfig, isTable bool) error {
	var errs []error
	errs = append(
		errs,
		validateEnrichLicenseTimerSignals(
			rowIdx,
			"expiry",
			&signals.Expiry,
			LicenseSignalExpiryTimestamp,
			LicenseSignalExpiryRemaining,
			isTable,
		),
	)
	errs = append(
		errs,
		validateEnrichLicenseTimerSignals(
			rowIdx,
			"authorization",
			&signals.Authorization,
			LicenseSignalAuthorizationTimestamp,
			LicenseSignalAuthorizationRemaining,
			isTable,
		),
	)
	errs = append(
		errs,
		validateEnrichLicenseTimerSignals(
			rowIdx,
			"certificate",
			&signals.Certificate,
			LicenseSignalCertificateTimestamp,
			LicenseSignalCertificateRemaining,
			isTable,
		),
	)
	errs = append(
		errs,
		validateEnrichLicenseTimerSignals(
			rowIdx,
			"grace",
			&signals.Grace,
			LicenseSignalGraceTimestamp,
			LicenseSignalGraceRemaining,
			isTable,
		),
	)
	errs = append(
		errs,
		validateEnrichLicenseValueKind(
			fmt.Sprintf("licensing[%d].signals.usage.used", rowIdx),
			&signals.Usage.Used,
			LicenseSignalUsageUsed,
			isTable,
		),
	)
	errs = append(
		errs,
		validateEnrichLicenseValueKind(
			fmt.Sprintf("licensing[%d].signals.usage.capacity", rowIdx),
			&signals.Usage.Capacity,
			LicenseSignalUsageCapacity,
			isTable,
		),
	)
	errs = append(
		errs,
		validateEnrichLicenseValueKind(
			fmt.Sprintf("licensing[%d].signals.usage.available", rowIdx),
			&signals.Usage.Available,
			LicenseSignalUsageAvailable,
			isTable,
		),
	)
	errs = append(
		errs,
		validateEnrichLicenseValueKind(
			fmt.Sprintf("licensing[%d].signals.usage.percent", rowIdx),
			&signals.Usage.Percent,
			LicenseSignalUsagePercent,
			isTable,
		),
	)
	return errors.Join(errs...)
}

func validateEnrichLicenseTimerSignals(
	rowIdx int,
	name string,
	signals *LicenseTimerSignalsConfig,
	timestampKind, remainingKind LicenseSignalKind,
	isTable bool,
) error {
	var errs []error
	basePath := fmt.Sprintf("licensing[%d].signals.%s", rowIdx, name)
	if signals.LicenseValueConfig.IsSet() && signals.Timestamp.IsSet() {
		errs = append(errs, fmt.Errorf("%s: inline timestamp and timestamp cannot both be set", basePath))
	}
	if (signals.LicenseValueConfig.IsSet() || signals.Timestamp.IsSet()) && signals.Remaining.IsSet() {
		errs = append(errs, fmt.Errorf("%s: timestamp and remaining cannot both be set", basePath))
	}
	errs = append(errs, validateEnrichLicenseValueKind(basePath, &signals.LicenseValueConfig, timestampKind, isTable))
	errs = append(
		errs,
		validateEnrichLicenseValueKind(basePath+".timestamp", &signals.Timestamp, timestampKind, isTable),
	)
	errs = append(
		errs,
		validateEnrichLicenseValueKind(basePath+".remaining", &signals.Remaining, remainingKind, isTable),
	)
	return errors.Join(errs...)
}

func validateEnrichLicenseValueKind(
	path string,
	value *LicenseValueConfig,
	defaultKind LicenseSignalKind,
	isTable bool,
) error {
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
	errs = append(errs, validateMapping(value.Mapping, metadataSymbol))

	if strings.HasPrefix(value.Name, "_") {
		errs = append(errs, fmt.Errorf("%s.name: name %q cannot be underscore-prefixed", path, value.Name))
	}
	if !isTable {
		if value.Index != 0 {
			errs = append(errs, fmt.Errorf("%s.index: scalar licensing values do not support `index` lookups", path))
		}
		if len(value.IndexTransform) > 0 {
			errs = append(
				errs,
				fmt.Errorf("%s.index_transform: scalar licensing values do not support `index_transform`", path),
			)
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

	errs = append(errs, validateEnrichSymbol(symbol, metadataSymbol))
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

func validateLicenseSignalDuplicates(
	rowIdx int,
	row *LicensingConfig,
	seen map[licenseSignalValidationKey]string,
) error {
	var errs []error
	identity := LicenseStructuralIdentity(*row)
	for _, sig := range collectLicenseSignalValues(*row) {
		if sig.kind == "" || !sig.value.IsSet() {
			continue
		}
		path := fmt.Sprintf("licensing[%d].%s", rowIdx, sig.path)
		key := licenseSignalValidationKey{
			identity: identity,
			kind:     sig.kind,
		}
		if prev, ok := seen[key]; ok {
			errs = append(
				errs,
				fmt.Errorf(
					"%s: duplicate signal kind %q for structural identity %q (first seen at %s)",
					path,
					sig.kind,
					identity,
					prev,
				),
			)
			continue
		}
		seen[key] = path
	}
	return errors.Join(errs...)
}

func validateLicenseSourceReferences(rowIdx int, row *LicensingConfig) error {
	if row.Table.OID == "" {
		return nil
	}

	var errs []error
	tableOID := TrimLicenseOID(row.Table.OID)
	for _, ref := range collectLicenseValueReferences(*row) {
		sourceOID, sourceField := valueSourceOID(ref.value.Symbol.OID, ref.value.From, ref.value.OID)
		if sourceOID == "" {
			continue
		}
		if !oidHasPrefix(TrimLicenseOID(sourceOID), tableOID) {
			errs = append(
				errs,
				fmt.Errorf(
					"licensing[%d].%s.%s: OID %q is outside table %q",
					rowIdx,
					ref.path,
					sourceField,
					sourceOID,
					row.Table.OID,
				),
			)
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
	visitLicenseSignalValues(&row, func(path string, value LicenseValueConfig) {
		if value.IsSet() {
			values = append(values, licenseSignalValueValidationRef{
				path:  path,
				kind:  value.Kind,
				value: value,
			})
		}
	}, nil)
	return values
}

func collectLicenseValueReferences(row LicensingConfig) []licenseValueValidationRef {
	var values []licenseValueValidationRef
	transformLicenseValues(&row, func(path string, value LicenseValueConfig) LicenseValueConfig {
		if value.IsSet() {
			values = append(values, licenseValueValidationRef{
				path:  path,
				value: value,
			})
		}
		return value
	})
	return values
}

func licenseRowHasSignalConfigs(row LicensingConfig) bool {
	var found bool
	ForEachLicenseSignalValue(row, func(LicenseValueConfig) {
		found = true
	})
	return found
}

func collectLicenseSignalSourceOIDs(row LicensingConfig) map[string]struct{} {
	oids := make(map[string]struct{})
	for _, sig := range collectLicenseSignalValues(row) {
		if oid := sig.value.SourceOID(); oid != "" {
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
