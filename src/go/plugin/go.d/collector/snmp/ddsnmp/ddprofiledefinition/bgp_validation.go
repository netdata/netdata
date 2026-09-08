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
		visitBGPSignalValues(&rows[i], nil, func(_ string, value BGPValueConfig) BGPValueConfig {
			normalizeBGPValue(&value)
			return value
		})
	}
}

func normalizeBGPValue(value *BGPValueConfig) {
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

func validateEnrichBGP(rows []BGPConfig) error {
	var errs []error
	seenSignals := make(map[bgpSignalValidationKey]string)

	for i := range rows {
		row := &rows[i]
		isTable := row.Table.OID != ""

		errs = append(errs, validateEnrichBGPRowShape(i, row))
		errs = append(
			errs,
			validateEnrichBGPValue(
				fmt.Sprintf("bgp[%d].identity.routing_instance", i),
				&row.Identity.RoutingInstance,
				isTable,
			),
		)
		errs = append(
			errs,
			validateEnrichBGPValue(fmt.Sprintf("bgp[%d].identity.neighbor", i), &row.Identity.Neighbor, isTable),
		)
		errs = append(
			errs,
			validateEnrichBGPValue(fmt.Sprintf("bgp[%d].identity.remote_as", i), &row.Identity.RemoteAS, isTable),
		)
		errs = append(
			errs,
			validateEnrichBGPAddressFamilyValue(
				fmt.Sprintf("bgp[%d].identity.address_family", i),
				&row.Identity.AddressFamily,
				isTable,
			),
		)
		errs = append(
			errs,
			validateEnrichBGPSubsequentAddressFamilyValue(
				fmt.Sprintf("bgp[%d].identity.subsequent_address_family", i),
				&row.Identity.SubsequentAddressFamily,
				isTable,
			),
		)
		errs = append(
			errs,
			validateEnrichBGPValue(
				fmt.Sprintf("bgp[%d].descriptors.local_address", i),
				&row.Descriptors.LocalAddress,
				isTable,
			),
		)
		errs = append(
			errs,
			validateEnrichBGPValue(fmt.Sprintf("bgp[%d].descriptors.local_as", i), &row.Descriptors.LocalAS, isTable),
		)
		errs = append(
			errs,
			validateEnrichBGPValue(
				fmt.Sprintf("bgp[%d].descriptors.local_identifier", i),
				&row.Descriptors.LocalIdentifier,
				isTable,
			),
		)
		errs = append(
			errs,
			validateEnrichBGPValue(
				fmt.Sprintf("bgp[%d].descriptors.peer_identifier", i),
				&row.Descriptors.PeerIdentifier,
				isTable,
			),
		)
		errs = append(
			errs,
			validateEnrichBGPValue(fmt.Sprintf("bgp[%d].descriptors.peer_type", i), &row.Descriptors.PeerType, isTable),
		)
		errs = append(
			errs,
			validateEnrichBGPValue(
				fmt.Sprintf("bgp[%d].descriptors.bgp_version", i),
				&row.Descriptors.BGPVersion,
				isTable,
			),
		)
		errs = append(
			errs,
			validateEnrichBGPValue(
				fmt.Sprintf("bgp[%d].descriptors.description", i),
				&row.Descriptors.Description,
				isTable,
			),
		)
		errs = append(errs, validateEnrichBGPSignalValues(i, row, isTable))
		errs = append(errs, validateBGPGroupKindCompatibility(i, row))
		errs = append(errs, validateBGPSourceReferences(i, row))
		errs = append(errs, validateBGPSignalDuplicates(i, row, seenSignals))

		errs = append(errs, validateEnrichMetricTags(fmt.Sprintf("bgp[%d]", i), row.MetricTags, !isTable))
	}

	errs = append(errs, validateBGPTableReferences(rows))

	return errors.Join(errs...)
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
			errs = append(
				errs,
				fmt.Errorf("bgp[%d].identity.neighbor: peer_family rows require neighbor identity", rowIdx),
			)
		}
		if !row.Identity.RemoteAS.IsSet() {
			errs = append(
				errs,
				fmt.Errorf("bgp[%d].identity.remote_as: peer_family rows require remote_as identity", rowIdx),
			)
		}
		if !row.Identity.AddressFamily.IsSet() {
			errs = append(
				errs,
				fmt.Errorf("bgp[%d].identity.address_family: peer_family rows require address_family identity", rowIdx),
			)
		}
		if !row.Identity.SubsequentAddressFamily.IsSet() {
			errs = append(
				errs,
				fmt.Errorf(
					"bgp[%d].identity.subsequent_address_family: peer_family rows require subsequent_address_family identity",
					rowIdx,
				),
			)
		}
	}

	if row.Table.OID == "" && row.ID == "" {
		sourceOIDs := collectBGPSignalSourceOIDs(*row)
		switch {
		case len(sourceOIDs) == 0:
			errs = append(
				errs,
				fmt.Errorf("bgp[%d]: scalar rows without a typed-field source OID require explicit id", rowIdx),
			)
		case len(sourceOIDs) > 1:
			errs = append(
				errs,
				fmt.Errorf("bgp[%d]: scalar rows with multiple typed-field source OIDs require explicit id", rowIdx),
			)
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
	visitBGPSignalValues(row, nil, func(path string, value BGPValueConfig) BGPValueConfig {
		if path == "state" || path == "previous_state" {
			return value
		}
		errs = append(errs, validateEnrichBGPValue(fmt.Sprintf("bgp[%d].%s", rowIdx, path), &value, isTable))
		return value
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

func validateEnrichBGPSubsequentAddressFamilyValue(
	path string,
	value *BGPSubsequentAddressFamilyValueConfig,
	isTable bool,
) error {
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
		errs = append(
			errs,
			fmt.Errorf("%s: must define value, from, symbol.OID, OID, index, index_from_end, or index_transform", path),
		)
	}
	errs = append(errs, validateMapping(value.Mapping, metadataSymbol))
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
			errs = append(
				errs,
				fmt.Errorf("%s.index_from_end: scalar BGP values do not support `index_from_end` lookups", path),
			)
		}
		if len(value.IndexTransform) > 0 {
			errs = append(
				errs,
				fmt.Errorf("%s.index_transform: scalar BGP values do not support `index_transform`", path),
			)
		}
	}
	if value.Table != "" && value.SourceOID() == "" {
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
	return fmt.Errorf(
		"%s: index, index_from_end, and index_transform are mutually exclusive (set: %s)",
		path,
		strings.Join(selectors, ", "),
	)
}

func validateEnrichBGPSymbol(path string, symbol *SymbolConfig) error {
	var errs []error

	errs = append(errs, validateEnrichSymbol(symbol, metadataSymbol))
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

	return errors.Join(errs...)
}

func validateBGPPeerStateMapping(path string, state *BGPStateConfig) error {
	mapping := state.EffectiveMapping()
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
	for raw, mapped := range value.EffectiveMapping().Items {
		if !valid(mapped) {
			errs = append(errs, fmt.Errorf("%s.mapping.items[%s]: invalid BGP %s %q", path, raw, label, mapped))
		}
	}
	return errors.Join(errs...)
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
		key := bgpSignalValidationKey{
			identity: identity,
			path:     path,
		}
		if prev, ok := seen[key]; ok {
			errs = append(
				errs,
				fmt.Errorf(
					"%s: duplicate BGP field for structural identity %q (first seen at %s)",
					fullPath,
					identity,
					prev,
				),
			)
			return
		}
		seen[key] = fullPath
	})
	return errors.Join(errs...)
}

func validateBGPSourceReferences(rowIdx int, row *BGPConfig) error {
	if row.Table.OID == "" {
		return nil
	}

	var errs []error
	tableOID := TrimBGPOID(row.Table.OID)
	for _, ref := range collectBGPValueReferences(*row) {
		if ref.value.Table != "" {
			continue
		}
		sourceOID, sourceField := valueSourceOID(ref.value.Symbol.OID, ref.value.From, ref.value.OID)
		if sourceOID == "" {
			continue
		}
		if !oidHasPrefix(TrimBGPOID(sourceOID), tableOID) {
			errs = append(
				errs,
				fmt.Errorf(
					"bgp[%d].%s.%s: OID %q is outside table %q",
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
			sourceOID := TrimBGPOID(ref.value.SourceOID())
			if sourceOID != "" && !oidHasPrefix(sourceOID, refTableOID) {
				errs = append(
					errs,
					fmt.Errorf(
						"bgp[%d].%s.table: referenced table %q uses OID %q; source OID %q is outside referenced table",
						rowIdx,
						ref.path,
						ref.value.Table,
						refTableOID,
						sourceOID,
					),
				)
			}
			lookupOID := TrimBGPOID(SymbolConfig(ref.value.LookupSymbol).OID)
			if lookupOID != "" && !oidHasPrefix(lookupOID, refTableOID) {
				errs = append(
					errs,
					fmt.Errorf(
						"bgp[%d].%s.lookup_symbol: referenced table %q uses OID %q; lookup OID %q is outside referenced table",
						rowIdx,
						ref.path,
						ref.value.Table,
						refTableOID,
						lookupOID,
					),
				)
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
			values = append(values, bgpValueValidationRef{
				path:  path,
				value: value,
			})
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
		if oid := value.SourceOID(); oid != "" {
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
