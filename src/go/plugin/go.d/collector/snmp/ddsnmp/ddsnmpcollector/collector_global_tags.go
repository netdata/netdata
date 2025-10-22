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

func (gc *globalTagsCollector) processStaticTags(staticTags []ddprofiledefinition.StaticMetricTagConfig, globalTags map[string]string) {
	ta := tagAdder{tags: globalTags}
	ta.addTags(parseStaticTags(staticTags))
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
