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

type Config struct {
	SnmpClient      gosnmp.Handler
	Profiles        []*ddsnmp.Profile
	Log             *logger.Logger
	SysObjectID     string
	DisableBulkWalk bool
}

func New(cfg Config) *Collector {
	coll := &Collector{
		log:         cfg.Log.With(slog.String("ddsnmp", "collector")),
		profiles:    make(map[string]*profileState),
		missingOIDs: make(map[string]bool),
		tableCache:  newTableCache(30*time.Minute, 1),
	}

	for _, prof := range cfg.Profiles {
		prof := prof
		handleCrossTableTagsWithoutMetrics(prof)
		coll.profiles[prof.SourceFile] = &profileState{profile: prof}
	}

	coll.globalTagsCollector = newGlobalTagsCollector(cfg.SnmpClient, coll.missingOIDs, coll.log)
	coll.deviceMetadataCollector = newDeviceMetadataCollector(cfg.SnmpClient, coll.missingOIDs, coll.log, cfg.SysObjectID)
	coll.scalarCollector = newScalarCollector(cfg.SnmpClient, coll.missingOIDs, coll.log)
	coll.tableCollector = newTableCollector(cfg.SnmpClient, coll.missingOIDs, coll.tableCache, coll.log, cfg.DisableBulkWalk)
	coll.vmetricsCollector = newVirtualMetricsCollector(coll.log)

	return coll
}

type (
	Collector struct {
		log         *logger.Logger
		profiles    map[string]*profileState
		missingOIDs map[string]bool
		tableCache  *tableCache

		globalTagsCollector     *globalTagsCollector
		deviceMetadataCollector *deviceMetadataCollector
		scalarCollector         *scalarCollector
		tableCollector          *tableCollector
		vmetricsCollector       *vmetricsCollector
	}
	profileState struct {
		profile        *ddsnmp.Profile
		initialized    bool
		globalTags     map[string]string
		deviceMetadata map[string]ddsnmp.MetaTag
	}
)

func (c *Collector) CollectDeviceMetadata() (map[string]ddsnmp.MetaTag, error) {
	meta := make(map[string]ddsnmp.MetaTag)

	for _, prof := range c.profiles {
		profDeviceMeta, err := c.deviceMetadataCollector.Collect(prof.profile)
		if err != nil {
			return nil, err
		}

		for k, v := range profDeviceMeta {
			mergeMetaTagIfAbsent(meta, k, v)
		}
	}

	return meta, nil
}

func (c *Collector) Collect() ([]*ddsnmp.ProfileMetrics, error) {
	var metrics []*ddsnmp.ProfileMetrics
	var errs []error

	if expired := c.tableCache.clearExpired(); len(expired) > 0 {
		c.log.Debugf("Cleared %d expired table cache entries", len(expired))
	}

	for _, prof := range c.profiles {
		pm, err := c.collectProfile(prof)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		c.updateProfileMetrics(pm)

		metrics = append(metrics, pm)

		if vmetrics := c.vmetricsCollector.Collect(prof.profile.Definition, pm.Metrics); len(vmetrics) > 0 {
			for i := range vmetrics {
				vmetrics[i].Profile = pm
			}

			pm.Metrics = slices.DeleteFunc(pm.Metrics, func(m ddsnmp.Metric) bool { return strings.HasPrefix(m.Name, "_") })
			pm.Metrics = append(pm.Metrics, vmetrics...)
		}
	}

	if len(metrics) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	if len(errs) > 0 {
		c.log.Debugf("collecting metrics: %v", errors.Join(errs...))
	}

	return metrics, nil
}

func (c *Collector) SetSNMPClient(snmpClient gosnmp.Handler) {
	if c.globalTagsCollector != nil {
		c.globalTagsCollector.snmpClient = snmpClient
	}
	if c.deviceMetadataCollector != nil {
		c.deviceMetadataCollector.snmpClient = snmpClient
	}
	if c.scalarCollector != nil {
		c.scalarCollector.snmpClient = snmpClient
	}
	if c.tableCollector != nil {
		c.tableCollector.snmpClient = snmpClient
	}
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

	tableMetrics, err := c.tableCollector.Collect(ps.profile)
	if err != nil {
		return nil, err
	}
	metrics = append(metrics, tableMetrics...)

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

func (c *Collector) updateProfileMetrics(pm *ddsnmp.ProfileMetrics) {
	for i := range pm.Metrics {
		m := &pm.Metrics[i]
		m.Description = metricMetaReplacer.Replace(m.Description)
		m.Family = metricMetaReplacer.Replace(m.Family)
		m.Unit = metricMetaReplacer.Replace(m.Unit)
		for k, v := range m.Tags {
			// Remove tags prefixed with "rm:", which are intended for temporary use during transforms
			// and should not appear in the final exported metric.
			if strings.HasPrefix(k, "rm:") {
				delete(m.Tags, k)
				continue
			}
			m.Tags[k] = metricMetaReplacer.Replace(v)
		}
	}
}

var metricMetaReplacer = strings.NewReplacer(
	"'", "",
	"\n", " ",
	"\r", " ",
	"\x00", "",
)

// handleCrossTableTagsWithoutMetrics ensures tables referenced only by cross-table tags
// are still walked during collection. Without this, if a table like ifXTable is used
// only for cross-table tags (e.g., getting interface names) but has no metrics defined,
// it won't be walked and the tags will be missing. This creates synthetic metric entries
// for such tables using the longest common OID prefix of the referenced columns.
func handleCrossTableTagsWithoutMetrics(prof *ddsnmp.Profile) {
	if prof.Definition == nil {
		return
	}

	seenTableNames := make(map[string]bool)

	for _, m := range prof.Definition.Metrics {
		seenTableNames[m.Table.Name] = true
	}

	tagCrossTableOnlyOIDs := make(map[string][]string)

	for _, m := range prof.Definition.Metrics {
		if m.IsScalar() {
			continue
		}
		for _, tag := range m.MetricTags {
			oid := tag.Symbol.OID
			if tag.Table == "" || seenTableNames[tag.Table] || oid == "" {
				continue
			}
			tagCrossTableOnlyOIDs[tag.Table] = append(tagCrossTableOnlyOIDs[tag.Table], oid)
		}
	}

	for tableName, oids := range tagCrossTableOnlyOIDs {
		slices.Sort(oids)
		oids = slices.Compact(oids)

		prof.Definition.Metrics = append(prof.Definition.Metrics, ddprofiledefinition.MetricsConfig{
			MIB: fmt.Sprintf("synthetic-%s-MIB", tableName),
			Table: ddprofiledefinition.SymbolConfig{
				OID:  longestCommonPrefix(oids),
				Name: tableName,
			},
		})
	}
}

func longestCommonPrefix(oids []string) string {
	if len(oids) == 0 {
		return ""
	}
	prefix := oids[0]
	for i := 1; i < len(oids); i++ {
		for !strings.HasPrefix(oids[i], prefix) {
			prefix = prefix[0 : len(prefix)-1]
			if len(prefix) == 0 {
				return ""
			}
		}
	}
	return strings.TrimSuffix(prefix, ".")
}
