// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func New(snmpClient gosnmp.Handler, profiles []*ddsnmp.Profile, log *logger.Logger) *Collector {
	coll := &Collector{
		log:         log.With(slog.String("ddsnmp", "collector")),
		snmpClient:  snmpClient,
		profiles:    make(map[string]*profileState),
		missingOIDs: make(map[string]bool),
		tableCache:  newTableCache(30*time.Minute, 1),
	}

	for _, prof := range profiles {
		prof := prof
		coll.profiles[prof.SourceFile] = &profileState{profile: prof}
	}

	coll.globalTagsCollector = NewGlobalTagsCollector(snmpClient, coll.missingOIDs, coll.log)
	coll.deviceMetadataCollector = NewDeviceMetadataCollector(snmpClient, coll.missingOIDs, coll.log)
	coll.scalarCollector = NewScalarCollector(snmpClient, coll.missingOIDs, coll.log)
	coll.tableCollector = NewTableCollector(snmpClient, coll.missingOIDs, coll.tableCache, coll.log)

	return coll
}

type (
	Collector struct {
		log         *logger.Logger
		snmpClient  gosnmp.Handler
		profiles    map[string]*profileState
		missingOIDs map[string]bool
		tableCache  *tableCache

		globalTagsCollector     *GlobalTagsCollector
		deviceMetadataCollector *DeviceMetadataCollector
		scalarCollector         *ScalarCollector
		tableCollector          *TableCollector

		DoTableMetrics bool
	}
	profileState struct {
		profile        *ddsnmp.Profile
		initialized    bool
		globalTags     map[string]string
		deviceMetadata map[string]string
	}
)

func (c *Collector) Collect() ([]*ddsnmp.ProfileMetrics, error) {
	var metrics []*ddsnmp.ProfileMetrics
	var errs []error

	if expired := c.tableCache.clearExpired(); len(expired) > 0 {
		c.log.Debugf("Cleared %d expired table cache entries", len(expired))
	}

	for _, prof := range c.profiles {
		if ms, err := c.collectProfile(prof); err != nil {
			errs = append(errs, err)
		} else if ms != nil {
			metrics = append(metrics, ms)
		}
	}

	if len(metrics) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	if len(errs) > 0 {
		c.log.Debugf("collecting metrics: %v", errors.Join(errs...))
	}

	c.updateMetrics(metrics)

	return metrics, nil
}

func (c *Collector) collectProfile(ps *profileState) (*ddsnmp.ProfileMetrics, error) {
	if !ps.initialized {
		globalTag, err := c.globalTagsCollector.Collect(ps.profile)
		if err != nil {
			return nil, fmt.Errorf("failed to collect global tags: %w", err)
		}

		deviceMeta, err := c.deviceMetadataCollector.Collect(ps.profile)
		if err != nil {
			return nil, fmt.Errorf("failed to collect device metadata: %w", err)
		}

		ps.globalTags = globalTag
		ps.deviceMetadata = deviceMeta
		ps.initialized = true
	}

	var metrics []ddsnmp.Metric

	scalarMetrics, err := c.scalarCollector.Collect(ps.profile)
	if err != nil {
		return nil, err
	}
	metrics = append(metrics, scalarMetrics...)

	if c.DoTableMetrics {
		tableMetrics, err := c.tableCollector.Collect(ps.profile)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, tableMetrics...)
	}

	pm := &ddsnmp.ProfileMetrics{
		Source:         ps.profile.SourceFile,
		DeviceMetadata: maps.Clone(ps.deviceMetadata),
		Tags:           maps.Clone(ps.globalTags),
		Metrics:        metrics,
	}
	for i := range pm.Metrics {
		pm.Metrics[i].Profile = pm
	}

	return pm, nil
}

func (c *Collector) updateMetrics(pms []*ddsnmp.ProfileMetrics) {
	// Find device vendor and type from any profile that has them.
	// Multiple profiles can be loaded for a single device (e.g., base profiles, generic MIB profiles),
	// but only device-specific profiles contain vendor/type information.
	// We need to apply vendor/type to ALL metrics across ALL profiles to ensure consistent
	// metric family naming (e.g., "interface/stats" → "router/cisco/interface/stats").
	for _, ps := range c.profiles {
		if !ps.initialized {
			continue
		}
		res, ok := ps.profile.Definition.Metadata["device"]
		if !ok {
			continue
		}
		dt, dv := res.Fields["type"].Value, res.Fields["vendor"].Value
		if dt == "" || dv == "" {
			continue
		}
		for _, pm := range pms {
			for i := range pm.Metrics {
				m := &pm.Metrics[i]
				m.Family = processMetricFamily(m.Family, dt, dv)
			}
		}
		break
	}

	for _, pm := range pms {
		for i := range pm.Metrics {
			m := &pm.Metrics[i]
			m.Description = metricMetaReplacer.Replace(m.Description)
			m.Family = metricMetaReplacer.Replace(m.Family)
			m.Unit = metricMetaReplacer.Replace(m.Unit)
			for k, v := range m.Tags {
				m.Tags[k] = metricMetaReplacer.Replace(v)
			}
		}
	}
}

func (c *Collector) snmpGet(oids []string) (map[string]gosnmp.SnmpPDU, error) {
	pdus := make(map[string]gosnmp.SnmpPDU)

	for chunk := range slices.Chunk(oids, c.snmpClient.MaxOids()) {
		result, err := c.snmpClient.Get(chunk)
		if err != nil {
			return nil, err
		}

		for _, pdu := range result.Variables {
			if !isPduWithData(pdu) {
				c.missingOIDs[trimOID(pdu.Name)] = true
				continue
			}
			pdus[trimOID(pdu.Name)] = pdu
		}
	}

	return pdus, nil
}

func processMetricFamily(family, devType, vendor string) string {
	prefix := strings.TrimPrefix(devType+"/"+vendor, "/")
	if prefix == "" {
		return family
	}
	if family == "" {
		return prefix
	}

	parts := strings.Split(family, "/")
	parts = slices.DeleteFunc(parts, func(s string) bool {
		return strings.EqualFold(s, devType) ||
			strings.EqualFold(s, devType+"s") ||
			strings.EqualFold(s, devType+"es") ||
			strings.EqualFold(s, vendor)
	})

	return strings.TrimSuffix(prefix+"/"+strings.Join(parts, "/"), "/")
}

var metricMetaReplacer = strings.NewReplacer(
	"'", "",
	"\n", " ",
	"\r", " ",
	"\x00", "",
)
