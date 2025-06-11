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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type (
	ProfileMetrics struct {
		Source         string
		DeviceMetadata map[string]string
		Tags           map[string]string
		Metrics        []Metric
	}
	Metric struct {
		Name        string
		Description string
		Family      string
		Unit        string
		MetricType  ddprofiledefinition.ProfileMetricType
		StaticTags  map[string]string
		Tags        map[string]string
		Mappings    map[int64]string
		IsTable     bool
		Value       int64
	}
)

func New(snmpClient gosnmp.Handler, profiles []*ddsnmp.Profile, log *logger.Logger) *Collector {
	coll := &Collector{
		log:         log.With(slog.String("ddsnmp", "collector")),
		snmpClient:  snmpClient,
		profiles:    make(map[string]*profileState),
		missingOIDs: make(map[string]bool),
		tableCache:  newTableCache(5*time.Minute, 0.2), // 5 min TTL with 20% jitter
	}

	for _, prof := range profiles {
		prof := prof
		coll.profiles[prof.SourceFile] = &profileState{profile: prof}
	}

	return coll
}

type (
	Collector struct {
		log         *logger.Logger
		snmpClient  gosnmp.Handler
		profiles    map[string]*profileState
		missingOIDs map[string]bool
		tableCache  *tableCache

		doTableMetrics bool
	}
	profileState struct {
		profile        *ddsnmp.Profile
		initialized    bool
		globalTags     map[string]string
		deviceMetadata map[string]string
	}
)

func (c *Collector) Collect() ([]*ProfileMetrics, error) {
	var metrics []*ProfileMetrics
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

	c.updateMetricFamily(metrics)
	cleanTags(metrics)

	return metrics, nil
}

func (c *Collector) collectProfile(ps *profileState) (*ProfileMetrics, error) {
	if !ps.initialized {
		globalTag, err := c.collectGlobalTags(ps.profile)
		if err != nil {
			return nil, fmt.Errorf("failed to collect global tags: %w", err)
		}

		deviceMeta, err := c.collectDeviceMetadata(ps.profile)
		if err != nil {
			return nil, fmt.Errorf("failed to collect device metadata: %w", err)
		}

		ps.globalTags = globalTag
		ps.deviceMetadata = deviceMeta
		ps.initialized = true
	}

	var metrics []Metric

	scalarMetrics, err := c.collectScalarMetrics(ps.profile)
	if err != nil {
		return nil, err
	}
	metrics = append(metrics, scalarMetrics...)

	if c.doTableMetrics {
		tableMetrics, err := c.collectTableMetrics(ps.profile)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, tableMetrics...)
	}

	return &ProfileMetrics{
		Source:         ps.profile.SourceFile,
		DeviceMetadata: maps.Clone(ps.deviceMetadata),
		Tags:           maps.Clone(ps.globalTags),
		Metrics:        metrics,
	}, nil
}

func (c *Collector) updateMetricFamily(pms []*ProfileMetrics) {
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
		return
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
	prefix := strings.TrimPrefix(devType+"s/"+vendor, "s/")
	if prefix == "" {
		return family
	}
	if family == "" {
		return prefix
	}

	parts := strings.Split(family, "/")
	parts = slices.DeleteFunc(parts, func(s string) bool {
		return strings.EqualFold(s, devType) || strings.EqualFold(s, devType+"s") || strings.EqualFold(s, vendor)
	})

	return strings.TrimSuffix(prefix+"/"+strings.Join(parts, "/"), "/")
}
