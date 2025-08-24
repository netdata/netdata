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

// deviceMetadataCollector handles collection of device metadata
type deviceMetadataCollector struct {
	snmpClient  gosnmp.Handler
	missingOIDs map[string]bool
	log         *logger.Logger
	sysobjectid string
}

func newDeviceMetadataCollector(snmpClient gosnmp.Handler, missingOIDs map[string]bool, log *logger.Logger, sysobjectid string) *deviceMetadataCollector {
	return &deviceMetadataCollector{
		snmpClient:  snmpClient,
		missingOIDs: missingOIDs,
		log:         log,
		sysobjectid: sysobjectid,
	}
}

func (dc *deviceMetadataCollector) Collect(prof *ddsnmp.Profile) (map[string]ddsnmp.MetaTag, error) {
	if len(prof.Definition.Metadata) == 0 && len(prof.Definition.SysobjectIDMetadata) == 0 {
		return nil, nil
	}

	resName := ddprofiledefinition.MetadataDeviceResource
	cfg, ok := prof.Definition.Metadata[resName]
	if !ok {
		return nil, nil
	}

	meta := make(map[string]ddsnmp.MetaTag)

	if dc.sysobjectid != "" {
		for i, entry := range prof.Definition.SysobjectIDMetadata {
			if ddsnmp.OidMatches(dc.sysobjectid, entry.SysobjectID) {
				if err := dc.processMetadataFields(entry.Metadata, meta, dc.sysobjectid == entry.SysobjectID); err != nil {
					dc.log.Warningf("sysobjectid_metadata[%d]: failed to process metadata fields for sysobjectid '%s': %v",
						i, entry.SysobjectID, err)
					continue
				}
				dc.log.Debugf("sysobjectid_metadata[%d]: matched sysobjectid '%s' with device OID '%s', applying metadata overrides",
					i, entry.SysobjectID, dc.sysobjectid)
			}
		}
	}

	if err := dc.processMetadataFields(cfg.Fields, meta, slices.Contains(prof.Definition.Extends, dc.sysobjectid)); err != nil {
		return ternary(len(meta) > 0, meta, nil), fmt.Errorf("failed to process metadata resource '%s': %w", resName, err)
	}

	return meta, nil
}

// processMetadataFields processes a single metadata resource
func (dc *deviceMetadataCollector) processMetadataFields(fields map[string]ddprofiledefinition.MetadataField, metadata map[string]ddsnmp.MetaTag, isExactMatch bool) error {
	oids := dc.collectStaticAndIdentifyOIDs(fields, metadata, isExactMatch)

	if len(oids) == 0 {
		return nil
	}

	pdus, err := dc.fetchMetadataValues(oids)
	if err != nil {
		return fmt.Errorf("failed to fetch metadata values: %w", err)
	}

	return dc.processDynamicFields(fields, pdus, metadata, isExactMatch)
}

// collectStaticAndIdentifyOIDs collects static values and returns OIDs to fetch
func (dc *deviceMetadataCollector) collectStaticAndIdentifyOIDs(fields map[string]ddprofiledefinition.MetadataField, metadata map[string]ddsnmp.MetaTag, isExactMatch bool) []string {
	var oids []string

	for name, field := range fields {
		switch {
		case field.Value != "":
			mergeMetaTagIfAbsent(metadata, name, ddsnmp.MetaTag{Value: field.Value, IsExactMatch: isExactMatch})
		case field.Symbol.OID != "":
			if !dc.missingOIDs[trimOID(field.Symbol.OID)] {
				oids = append(oids, field.Symbol.OID)
			}
		case len(field.Symbols) > 0:
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

func (dc *deviceMetadataCollector) fetchMetadataValues(oids []string) (map[string]gosnmp.SnmpPDU, error) {
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

func (dc *deviceMetadataCollector) processDynamicFields(fields map[string]ddprofiledefinition.MetadataField, pdus map[string]gosnmp.SnmpPDU, metadata map[string]ddsnmp.MetaTag, isExactMatch bool) error {
	var errs []error

	for name, field := range fields {
		switch {
		case field.Symbol.OID != "":
			// Single symbol
			v, err := dc.processSymbolValue(field.Symbol, pdus, true)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to process metadata field '%s': %w", name, err))
				continue
			}
			if v != "" {
				mergeMetaTagIfAbsent(metadata, name, ddsnmp.MetaTag{Value: v, IsExactMatch: isExactMatch})
			}
		case len(field.Symbols) > 0:
			// Multiple symbols - try each until one succeeds
			for i, sym := range field.Symbols {
				v, err := dc.processSymbolValue(sym, pdus, i == len(field.Symbols)-1)
				if err != nil {
					errs = append(errs, fmt.Errorf("failed to process metadata field '%s' symbol '%s': %w",
						name, sym.Name, err))
					continue
				}
				if v != "" {
					mergeMetaTagIfAbsent(metadata, name, ddsnmp.MetaTag{Value: v, IsExactMatch: isExactMatch})
					break // Use first successful value
				}
			}
		}
	}

	if len(errs) > 0 && len(metadata) == 0 {
		return fmt.Errorf("failed to process any metadata fields: %w", errors.Join(errs...))
	}

	return nil
}

func (dc *deviceMetadataCollector) processSymbolValue(cfg ddprofiledefinition.SymbolConfig, pdus map[string]gosnmp.SnmpPDU, lastSymbol bool) (string, error) {
	pdu, ok := pdus[trimOID(cfg.OID)]
	if !ok {
		return "", nil
	}

	val, err := convPduToStringf(pdu, cfg.Format)
	if err != nil {
		return "", err
	}

	if cfg.ExtractValueCompiled != nil {
		sm := cfg.ExtractValueCompiled.FindStringSubmatch(val)
		if len(sm) == 0 && !lastSymbol {
			return "", nil
		} else if len(sm) > 1 {
			val = sm[1]
		}
		// Note: If extract_value doesn't match, we still use the original value
		// This is intentional as extract_value is for extracting a portion of the value
	}

	if cfg.MatchPatternCompiled != nil {
		sm := cfg.MatchPatternCompiled.FindStringSubmatch(val)
		if len(sm) == 0 {
			// Pattern didn't match - return empty string to indicate no match
			// When match_pattern is specified, we only use the value if it matches
			return "", nil
		}
		val = replaceSubmatches(cfg.MatchValue, sm)
	}

	if v, ok := cfg.Mapping[val]; ok {
		val = v
	}

	return val, nil
}
