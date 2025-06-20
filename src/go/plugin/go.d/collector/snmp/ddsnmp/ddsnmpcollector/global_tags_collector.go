// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// GlobalTagsCollector handles collection of profile-wide tags
type GlobalTagsCollector struct {
	snmpClient   gosnmp.Handler
	missingOIDs  map[string]bool
	log          *logger.Logger
	tagProcessor *TagProcessorFactory
}

// NewGlobalTagsCollector creates a new global tags collector
func NewGlobalTagsCollector(snmpClient gosnmp.Handler, missingOIDs map[string]bool, log *logger.Logger) *GlobalTagsCollector {
	return &GlobalTagsCollector{
		snmpClient:   snmpClient,
		missingOIDs:  missingOIDs,
		log:          log,
		tagProcessor: NewTagProcessorFactory(),
	}
}

// Collect gathers all global tags from the profile
func (gc *GlobalTagsCollector) Collect(prof *ddsnmp.Profile) (map[string]string, error) {
	if len(prof.Definition.MetricTags) == 0 && len(prof.Definition.StaticTags) == 0 {
		return nil, nil
	}

	globalTags := make(map[string]string)

	// Process static tags first
	gc.processStaticTags(prof.Definition.StaticTags, globalTags)

	// Process dynamic tags
	if err := gc.processDynamicTags(prof.Definition.MetricTags, globalTags); err != nil {
		return globalTags, err
	}

	return globalTags, nil
}

// processStaticTags processes static tags from the profile
func (gc *GlobalTagsCollector) processStaticTags(staticTags []string, globalTags map[string]string) {
	for _, tag := range staticTags {
		if n, v, _ := strings.Cut(tag, ":"); n != "" && v != "" {
			globalTags[n] = v
		}
	}
}

// processDynamicTags processes tags that require SNMP fetching
func (gc *GlobalTagsCollector) processDynamicTags(metricTags []ddprofiledefinition.MetricTagConfig, globalTags map[string]string) error {
	// Identify OIDs to collect
	oids, missingOIDs := gc.identifyTagOIDs(metricTags)

	if len(missingOIDs) > 0 {
		gc.log.Debugf("global tags missing OIDs: %v", missingOIDs)
	}

	if len(oids) == 0 {
		return nil
	}

	// Fetch tag values
	pdus, err := gc.fetchTagValues(oids)
	if err != nil {
		return fmt.Errorf("failed to fetch global tag values: %w", err)
	}

	// Process each tag configuration
	var errs []error
	for _, tagCfg := range metricTags {
		if tagCfg.Symbol.OID == "" {
			continue
		}

		tagValues, err := gc.processTagValue(tagCfg, pdus)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to process tag value for '%s/%s': %w",
				tagCfg.Tag, tagCfg.Symbol.Name, err))
			continue
		}

		mergeTagsWithEmptyFallback(globalTags, tagValues)
	}

	if len(errs) > 0 && len(globalTags) == 0 {
		return fmt.Errorf("failed to process any global tags: %w", errors.Join(errs...))
	}

	return nil
}

// identifyTagOIDs returns OIDs to collect and OIDs that are known to be missing
func (gc *GlobalTagsCollector) identifyTagOIDs(metricTags []ddprofiledefinition.MetricTagConfig) ([]string, []string) {
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

// fetchTagValues retrieves values for the given OIDs
func (gc *GlobalTagsCollector) fetchTagValues(oids []string) (map[string]gosnmp.SnmpPDU, error) {
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

// processTagValue processes a single tag configuration
func (gc *GlobalTagsCollector) processTagValue(tagCfg ddprofiledefinition.MetricTagConfig, pdus map[string]gosnmp.SnmpPDU) (map[string]string, error) {
	return processMetricTagValue(tagCfg, pdus)
}
