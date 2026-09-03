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
func (gc *globalTagsCollector) collect(prof *ddsnmp.Profile) (map[string]string, error) {
	return gc.collectObserved(prof, &ddsnmp.CollectionStats{}, nil)
}

func (gc *globalTagsCollector) collectObserved(
	prof *ddsnmp.Profile,
	stats *ddsnmp.CollectionStats,
	acquisition *acquisitionProfileCollection,
) (map[string]string, error) {
	if len(prof.Definition.MetricTags) == 0 && len(prof.Definition.StaticTags) == 0 {
		return nil, nil
	}

	tags := make(map[string]string)

	gc.processStaticTags(prof.Definition.StaticTags, tags)

	observer := acquisition.globalTagObserver(prof.Definition.MetricTags)
	if err := gc.processDynamicTagsObserved(prof.Definition.MetricTags, tags, stats, observer); err != nil {
		return ternary(len(tags) > 0, tags, nil), err
	}

	return tags, nil
}

func (gc *globalTagsCollector) processStaticTags(staticTags []ddprofiledefinition.StaticMetricTagConfig, globalTags map[string]string) {
	ta := tagAdder{tags: globalTags}
	ta.addTags(parseStaticTags(staticTags))
}

// processDynamicTags processes tags that require SNMP fetching
func (gc *globalTagsCollector) processDynamicTags(metricTags []ddprofiledefinition.GlobalMetricTagConfig, globalTags map[string]string) error {
	return gc.processDynamicTagsObserved(metricTags, globalTags, &ddsnmp.CollectionStats{}, nil)
}

func (gc *globalTagsCollector) processDynamicTagsObserved(
	metricTags []ddprofiledefinition.GlobalMetricTagConfig,
	globalTags map[string]string,
	stats *ddsnmp.CollectionStats,
	observer *acquisitionGlobalTagObserver,
) error {
	// Identify OIDs to collect
	oids, missingOIDs := gc.identifyTagOIDs(metricTags)
	stats.Errors.MissingOIDs += int64(len(missingOIDs))
	observer.start(gc.missingOIDs)

	if len(missingOIDs) > 0 {
		gc.log.Debugf("global tags missing OIDs: %v", missingOIDs)
	}

	if len(oids) == 0 {
		return nil
	}

	pdus, err := gc.fetchTagValues(oids, stats)
	if err != nil {
		observer.failUnfinished(AcquisitionFailureClassTransport)
		return fmt.Errorf("failed to fetch global tag values: %w", err)
	}
	observer.start(gc.missingOIDs)

	// Collect each tag configuration
	var errs []error
	for i, tagCfg := range metricTags {
		cfg := tagCfg.MetricTagConfig
		if cfg.Symbol.OID == "" {
			continue
		}

		ta := tagAdder{tags: globalTags}

		var observed bool
		var err error
		if observer == nil {
			err = gc.tagProc.processTag(cfg, pdus, ta)
		} else {
			observed, err = gc.tagProc.processTagObserved(cfg, pdus, ta)
		}
		if err != nil {
			stats.Errors.Processing.Preparation++
			observer.rejected(i)
			errs = append(errs, fmt.Errorf("failed to process tag value for %q: %w",
				metricTagDisplayName(cfg), err))
			continue
		}
		if observed {
			observer.value(i)
		} else {
			observer.empty(i)
		}
	}

	if len(errs) > 0 && len(globalTags) == 0 {
		return fmt.Errorf("failed to process any global tags: %w", errors.Join(errs...))
	}

	return nil
}

func (gc *globalTagsCollector) identifyTagOIDs(metricTags []ddprofiledefinition.GlobalMetricTagConfig) ([]string, []string) {
	var oids []string
	var missingOIDs []string

	for _, tagCfg := range metricTags {
		cfg := tagCfg.MetricTagConfig
		if cfg.Symbol.OID == "" {
			continue
		}

		oid := trimOID(cfg.Symbol.OID)
		if gc.missingOIDs[oid] {
			missingOIDs = append(missingOIDs, cfg.Symbol.OID)
			continue
		}

		oids = append(oids, cfg.Symbol.OID)
	}

	// Sort and deduplicate
	slices.Sort(oids)
	oids = slices.Compact(oids)

	return oids, missingOIDs
}

func (gc *globalTagsCollector) fetchTagValues(oids []string, stats *ddsnmp.CollectionStats) (map[string]gosnmp.SnmpPDU, error) {
	return getSNMPValues(gc.snmpClient, oids, gc.missingOIDs, stats)
}
