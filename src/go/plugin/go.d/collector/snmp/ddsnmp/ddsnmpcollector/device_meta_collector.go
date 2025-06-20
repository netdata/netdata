// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"slices"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// DeviceMetadataCollector handles collection of device metadata
type DeviceMetadataCollector struct {
	snmpClient  gosnmp.Handler
	missingOIDs map[string]bool
	log         *logger.Logger
}

// NewDeviceMetadataCollector creates a new device metadata collector
func NewDeviceMetadataCollector(snmpClient gosnmp.Handler, missingOIDs map[string]bool, log *logger.Logger) *DeviceMetadataCollector {
	return &DeviceMetadataCollector{
		snmpClient:  snmpClient,
		missingOIDs: missingOIDs,
		log:         log,
	}
}

// Collect gathers device metadata from the profile
func (dc *DeviceMetadataCollector) Collect(prof *ddsnmp.Profile) (map[string]string, error) {
	if len(prof.Definition.Metadata) == 0 {
		return nil, nil
	}

	metadata := make(map[string]string)

	// Process each metadata resource
	for resName, cfg := range prof.Definition.Metadata {
		if !ddprofiledefinition.IsMetadataResourceWithScalarOids(resName) {
			continue
		}

		if err := dc.processResource(resName, cfg, metadata); err != nil {
			return metadata, fmt.Errorf("failed to process metadata resource '%s': %w", resName, err)
		}
	}

	return metadata, nil
}

// processResource processes a single metadata resource
func (dc *DeviceMetadataCollector) processResource(resName string, cfg ddprofiledefinition.MetadataResourceConfig, metadata map[string]string) error {
	// First pass: collect static values and identify OIDs
	staticValues := make(map[string]string)
	oids := dc.collectStaticAndIdentifyOIDs(cfg, staticValues)

	// Add static values to metadata
	for k, v := range staticValues {
		metadata[k] = v
	}

	if len(oids) == 0 {
		return nil
	}

	// Fetch dynamic values
	pdus, err := dc.fetchMetadataValues(oids)
	if err != nil {
		return fmt.Errorf("failed to fetch metadata values: %w", err)
	}

	// Process dynamic fields
	return dc.processDynamicFields(cfg, pdus, metadata)
}

// collectStaticAndIdentifyOIDs collects static values and returns OIDs to fetch
func (dc *DeviceMetadataCollector) collectStaticAndIdentifyOIDs(cfg ddprofiledefinition.MetadataResourceConfig, staticValues map[string]string) []string {
	var oids []string

	for name, field := range cfg.Fields {
		switch {
		case field.Value != "":
			// Static value
			staticValues[name] = field.Value
		case field.Symbol.OID != "":
			// Single symbol
			if !dc.missingOIDs[trimOID(field.Symbol.OID)] {
				oids = append(oids, field.Symbol.OID)
			}
		case len(field.Symbols) > 0:
			// Multiple symbols
			for _, sym := range field.Symbols {
				if sym.OID != "" && !dc.missingOIDs[trimOID(sym.OID)] {
					oids = append(oids, sym.OID)
				}
			}
		}
	}

	// Sort and deduplicate
	slices.Sort(oids)
	oids = slices.Compact(oids)

	return oids
}

// fetchMetadataValues retrieves values for the given OIDs
func (dc *DeviceMetadataCollector) fetchMetadataValues(oids []string) (map[string]gosnmp.SnmpPDU, error) {
	pdus := make(map[string]gosnmp.SnmpPDU)
	maxOids := dc.snmpClient.MaxOids()

	for chunk := range slices.Chunk(oids, maxOids) {
		result, err := dc.snmpClient.Get(chunk)
		if err != nil {
			return nil, err
		}

		for _, pdu := range result.Variables {
			if !isPduWithData(pdu) {
				dc.missingOIDs[trimOID(pdu.Name)] = true
				continue
			}
			pdus[trimOID(pdu.Name)] = pdu
		}
	}

	return pdus, nil
}

// processDynamicFields processes fields that require SNMP values
func (dc *DeviceMetadataCollector) processDynamicFields(cfg ddprofiledefinition.MetadataResourceConfig, pdus map[string]gosnmp.SnmpPDU, metadata map[string]string) error {
	var errs []error

	for name, field := range cfg.Fields {
		switch {
		case field.Symbol.OID != "":
			// Single symbol
			v, err := dc.processSymbolValue(field.Symbol, pdus)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to process metadata field '%s': %w", name, err))
				continue
			}
			mergeTagsWithEmptyFallback(metadata, map[string]string{name: v})

		case len(field.Symbols) > 0:
			// Multiple symbols - try each until one succeeds
			for _, sym := range field.Symbols {
				v, err := dc.processSymbolValue(sym, pdus)
				if err != nil {
					errs = append(errs, fmt.Errorf("failed to process metadata field '%s' symbol '%s': %w",
						name, sym.Name, err))
					continue
				}
				mergeTagsWithEmptyFallback(metadata, map[string]string{name: v})
				break // Use first successful value
			}
		}
	}

	if len(errs) > 0 && len(metadata) == 0 {
		return fmt.Errorf("failed to process any metadata fields: %w", errors.Join(errs...))
	}

	return nil
}

// processSymbolValue processes a single symbol configuration
func (dc *DeviceMetadataCollector) processSymbolValue(cfg ddprofiledefinition.SymbolConfig, pdus map[string]gosnmp.SnmpPDU) (string, error) {
	pdu, ok := pdus[trimOID(cfg.OID)]
	if !ok {
		return "", nil
	}

	val, err := convPduToStringf(pdu, cfg.Format)
	if err != nil {
		return "", err
	}

	// Apply extract_value pattern
	if cfg.ExtractValueCompiled != nil {
		if sm := cfg.ExtractValueCompiled.FindStringSubmatch(val); len(sm) > 1 {
			val = sm[1]
		}
	}

	// Apply match_pattern and match_value
	if cfg.MatchPatternCompiled != nil {
		if sm := cfg.MatchPatternCompiled.FindStringSubmatch(val); len(sm) > 0 {
			val = replaceSubmatches(cfg.MatchValue, sm)
		}
	}

	// Apply mapping
	if v, ok := cfg.Mapping[val]; ok {
		val = v
	}

	return val, nil
}
