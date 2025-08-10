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

// globalTagsCollector handles collection of profile-wide tags
type globalTagsCollector struct {
	snmpClient  gosnmp.Handler
	missingOIDs map[string]bool
	log         *logger.Logger
	tagProc     *globalTagProcessor
}

func newGlobalTagsCollector(snmpClient gosnmp.Handler, missingOIDs map[string]bool, log *logger.Logger) *globalTagsCollector {
	return &globalTagsCollector{
		snmpClient:  snmpClient,
		missingOIDs: missingOIDs,
		log:         log,
		tagProc:     newGlobalTagProcessor(),
	}
}

// Collect gathers all global tags from the profile
func (gc *globalTagsCollector) Collect(prof *ddsnmp.Profile) (map[string]string, error) {
	if len(prof.Definition.MetricTags) == 0 && len(prof.Definition.StaticTags) == 0 {
		return nil, nil
	}

	tags := make(map[string]string)

	gc.processStaticTags(prof.Definition.StaticTags, tags)

	if err := gc.processDynamicTags(prof.Definition.MetricTags, tags); err != nil {
		return ternary(len(tags) > 0, tags, nil), err
	}

	return tags, nil
}

func (gc *globalTagsCollector) processStaticTags(staticTags []string, globalTags map[string]string) {
	mergeTagsWithEmptyFallback(globalTags, parseStaticTags(staticTags))
}

// processDynamicTags processes tags that require SNMP fetching
func (gc *globalTagsCollector) processDynamicTags(metricTags []ddprofiledefinition.MetricTagConfig, globalTags map[string]string) error {
	// Identify OIDs to collect
	oids, missingOIDs := gc.identifyTagOIDs(metricTags)

	if len(missingOIDs) > 0 {
		gc.log.Debugf("global tags missing OIDs: %v", missingOIDs)
	}

	if len(oids) == 0 {
		return nil
	}

	pdus, err := gc.fetchTagValues(oids)
	if err != nil {
		return fmt.Errorf("failed to fetch global tag values: %w", err)
	}

	// Collect each tag configuration
	var errs []error
	for _, tagCfg := range metricTags {
		if tagCfg.Symbol.OID == "" {
			continue
		}

		ta := tagAdder{tags: globalTags}

		if err := gc.tagProc.processTag(tagCfg, pdus, ta); err != nil {
			errs = append(errs, fmt.Errorf("failed to process tag value for '%s/%s': %w",
				tagCfg.Tag, tagCfg.Symbol.Name, err))
			continue
		}
	}

	if len(errs) > 0 && len(globalTags) == 0 {
		return fmt.Errorf("failed to process any global tags: %w", errors.Join(errs...))
	}

	return nil
}

func (gc *globalTagsCollector) identifyTagOIDs(metricTags []ddprofiledefinition.MetricTagConfig) ([]string, []string) {
	var oids []string
	var missingOIDs []string

	for _, tagCfg := range metricTags {
		if tagCfg.Symbol.OID == "" {
			continue
		}

		oid := trimOID(tagCfg.Symbol.OID)
		if gc.missingOIDs[oid] {
			missingOIDs = append(missingOIDs, tagCfg.Symbol.OID)
			continue
		}

		oids = append(oids, tagCfg.Symbol.OID)
	}

	// Sort and deduplicate
	slices.Sort(oids)
	oids = slices.Compact(oids)

	return oids, missingOIDs
}

func (gc *globalTagsCollector) fetchTagValues(oids []string) (map[string]gosnmp.SnmpPDU, error) {
	pdus := make(map[string]gosnmp.SnmpPDU)
	maxOids := gc.snmpClient.MaxOids()

	for chunk := range slices.Chunk(oids, maxOids) {
		result, err := gc.snmpClient.Get(chunk)
		if err != nil {
			return nil, err
		}

		for _, pdu := range result.Variables {
			if !isPduWithData(pdu) {
				gc.missingOIDs[trimOID(pdu.Name)] = true
				continue
			}
			pdus[trimOID(pdu.Name)] = pdu
		}
	}

	return pdus, nil
}

// deviceMetadataCollector handles collection of device metadata
type deviceMetadataCollector struct {
	snmpClient  gosnmp.Handler
	missingOIDs map[string]bool
	log         *logger.Logger
}

func newDeviceMetadataCollector(snmpClient gosnmp.Handler, missingOIDs map[string]bool, log *logger.Logger) *deviceMetadataCollector {
	return &deviceMetadataCollector{
		snmpClient:  snmpClient,
		missingOIDs: missingOIDs,
		log:         log,
	}
}

func (dc *deviceMetadataCollector) Collect(prof *ddsnmp.Profile) (map[string]string, error) {
	if len(prof.Definition.Metadata) == 0 {
		return nil, nil
	}

	meta := make(map[string]string)

	for resName, cfg := range prof.Definition.Metadata {
		if !ddprofiledefinition.IsMetadataResourceWithScalarOids(resName) {
			continue
		}

		if err := dc.processResource(cfg, meta); err != nil {
			return ternary(len(meta) > 0, meta, nil), fmt.Errorf("failed to process metadata resource '%s': %w", resName, err)
		}
	}

	return meta, nil
}

// processResource processes a single metadata resource
func (dc *deviceMetadataCollector) processResource(cfg ddprofiledefinition.MetadataResourceConfig, metadata map[string]string) error {
	staticValues := make(map[string]string)
	oids := dc.collectStaticAndIdentifyOIDs(cfg, staticValues)

	for k, v := range staticValues {
		metadata[k] = v
	}

	if len(oids) == 0 {
		return nil
	}

	pdus, err := dc.fetchMetadataValues(oids)
	if err != nil {
		return fmt.Errorf("failed to fetch metadata values: %w", err)
	}

	return dc.processDynamicFields(cfg, pdus, metadata)
}

// collectStaticAndIdentifyOIDs collects static values and returns OIDs to fetch
func (dc *deviceMetadataCollector) collectStaticAndIdentifyOIDs(cfg ddprofiledefinition.MetadataResourceConfig, staticValues map[string]string) []string {
	var oids []string

	for name, field := range cfg.Fields {
		switch {
		case field.Value != "":
			staticValues[name] = field.Value
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

func (dc *deviceMetadataCollector) processDynamicFields(cfg ddprofiledefinition.MetadataResourceConfig, pdus map[string]gosnmp.SnmpPDU, metadata map[string]string) error {
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
			if v != "" {
				mergeTagsWithEmptyFallback(metadata, map[string]string{name: v})
			}
		case len(field.Symbols) > 0:
			// Multiple symbols - try each until one succeeds
			for _, sym := range field.Symbols {
				v, err := dc.processSymbolValue(sym, pdus)
				if err != nil {
					errs = append(errs, fmt.Errorf("failed to process metadata field '%s' symbol '%s': %w",
						name, sym.Name, err))
					continue
				}
				if v != "" {
					mergeTagsWithEmptyFallback(metadata, map[string]string{name: v})
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

func (dc *deviceMetadataCollector) processSymbolValue(cfg ddprofiledefinition.SymbolConfig, pdus map[string]gosnmp.SnmpPDU) (string, error) {
	pdu, ok := pdus[trimOID(cfg.OID)]
	if !ok {
		return "", nil
	}

	val, err := convPduToStringf(pdu, cfg.Format)
	if err != nil {
		return "", err
	}

	if cfg.ExtractValueCompiled != nil {
		if sm := cfg.ExtractValueCompiled.FindStringSubmatch(val); len(sm) > 1 {
			val = sm[1]
		}
	}

	if cfg.MatchPatternCompiled != nil {
		if sm := cfg.MatchPatternCompiled.FindStringSubmatch(val); len(sm) > 0 {
			val = replaceSubmatches(cfg.MatchValue, sm)
		}
	}

	if v, ok := cfg.Mapping[val]; ok {
		val = v
	}

	return val, nil
}
