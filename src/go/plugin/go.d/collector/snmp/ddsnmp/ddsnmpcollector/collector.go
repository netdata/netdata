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
		profile     *ddsnmp.Profile
		initialized bool
		cache       struct {
			globalTags     map[string]string
			deviceMetadata map[string]ddsnmp.MetaTag
		}
	}
)

func (c *Collector) CollectDeviceMetadata() (map[string]ddsnmp.MetaTag, error) {
	meta := make(map[string]ddsnmp.MetaTag)

	for _, prof := range c.profiles {
		profDeviceMeta, err := c.deviceMetadataCollector.collect(prof.profile)
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

	expired := c.tableCache.clearExpired()
	if len(expired) > 0 {
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

		now := time.Now()
		if vmetrics := c.vmetricsCollector.collect(prof.profile.Definition, pm.Metrics); len(vmetrics) > 0 {
			for i := range vmetrics {
				vmetrics[i].Profile = pm
			}

			pm.Metrics = append(pm.Metrics, vmetrics...)
			pm.Stats.Metrics.Virtual += int64(len(vmetrics))
			pm.Stats.Timing.VirtualMetrics = time.Since(now)
		}

		pm.HiddenMetrics = collectHiddenMetrics(pm.Metrics)
		pm.Metrics = slices.DeleteFunc(pm.Metrics, func(m ddsnmp.Metric) bool { return strings.HasPrefix(m.Name, "_") })
	}

	if len(metrics) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	if len(errs) > 0 {
		c.log.Debugf("collecting metrics: %v", errors.Join(errs...))
	}

	return metrics, nil
}

func collectHiddenMetrics(metrics []ddsnmp.Metric) []ddsnmp.Metric {
	var hidden []ddsnmp.Metric
	for _, metric := range metrics {
		if strings.HasPrefix(metric.Name, "_") {
			hidden = append(hidden, metric)
		}
	}
	return hidden
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
	pm := &ddsnmp.ProfileMetrics{
		Source: ps.profile.SourceFile,
	}

	if !ps.initialized {
		globalTag, err := c.globalTagsCollector.collect(ps.profile)
		if err != nil {
			return nil, fmt.Errorf("failed to collect global tags: %w", err)
		}
		ps.cache.globalTags = globalTag

		deviceMeta, err := c.deviceMetadataCollector.collect(ps.profile)
		if err != nil {
			return nil, fmt.Errorf("failed to collect device metadata: %w", err)
		}
		ps.cache.deviceMetadata = deviceMeta

		ps.initialized = true
	}

	pm.Tags = maps.Clone(ps.cache.globalTags)
	pm.DeviceMetadata = maps.Clone(ps.cache.deviceMetadata)

	now := time.Now()
	scalarMetrics, err := c.scalarCollector.collect(ps.profile, &pm.Stats)
	if err != nil {
		return nil, err
	}
	pm.Metrics = append(pm.Metrics, scalarMetrics...)
	pm.Stats.Timing.Scalar = time.Since(now)
	pm.Stats.Metrics.Scalar += int64(len(scalarMetrics))

	now = time.Now()
	tableMetrics, err := c.tableCollector.collect(ps.profile, &pm.Stats)
	if err != nil {
		return nil, err
	}
	pm.Metrics = append(pm.Metrics, tableMetrics...)
	pm.Stats.Timing.Table = time.Since(now)
	pm.Stats.Metrics.Table += int64(len(tableMetrics))

	topologyMetrics, err := c.collectTopologyMetrics(ps.profile, &pm.Stats)
	if err != nil {
		return nil, err
	}
	pm.TopologyMetrics = append(pm.TopologyMetrics, topologyMetrics...)

	now = time.Now()
	licenseRows, err := c.collectLicenseRows(ps.profile, &pm.Stats)
	if err != nil {
		c.log.Limit(licenseRowsFailedLogKey+ps.profile.SourceFile, 1, licenseRowsErrorLogEvery).
			Warningf("failed to collect licensing rows for profile %q: %v", ps.profile.SourceFile, err)
	}
	pm.LicenseRows = append(pm.LicenseRows, licenseRows...)
	pm.Stats.Metrics.Licensing += int64(len(licenseRows))
	pm.Stats.Timing.Licensing = time.Since(now)

	now = time.Now()
	bgpRows, err := c.collectBGPRows(ps.profile, &pm.Stats)
	if err != nil {
		pm.BGPCollectError = err
		c.log.Limit(bgpRowsFailedLogKey+ps.profile.SourceFile, 1, bgpRowsErrorLogEvery).
			Warningf("failed to collect BGP rows for profile %q: %v", ps.profile.SourceFile, err)
	}
	pm.BGPRows = append(pm.BGPRows, bgpRows...)
	pm.Stats.Metrics.BGP += int64(len(bgpRows))
	pm.Stats.Timing.BGP = time.Since(now)

	for i := range pm.Metrics {
		pm.Metrics[i].Profile = pm
	}
	for i := range pm.TopologyMetrics {
		pm.TopologyMetrics[i].Profile = pm
	}

	return pm, nil
}

func (c *Collector) updateProfileMetrics(pm *ddsnmp.ProfileMetrics) {
	for i := range pm.Metrics {
		sanitizeMetricMetadata(&pm.Metrics[i])
	}
	for i := range pm.TopologyMetrics {
		sanitizeMetricMetadata(&pm.TopologyMetrics[i])
	}
	for i := range pm.LicenseRows {
		sanitizeLicenseRow(&pm.LicenseRows[i])
	}
	for i := range pm.BGPRows {
		sanitizeBGPRow(&pm.BGPRows[i])
	}
}

func sanitizeMetricMetadata(m *ddsnmp.Metric) {
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

func sanitizeLicenseRow(row *ddsnmp.LicenseRow) {
	row.ID = metricMetaReplacer.Replace(row.ID)
	row.Name = metricMetaReplacer.Replace(row.Name)
	row.Feature = metricMetaReplacer.Replace(row.Feature)
	row.Component = metricMetaReplacer.Replace(row.Component)
	row.Type = metricMetaReplacer.Replace(row.Type)
	row.Impact = metricMetaReplacer.Replace(row.Impact)
	row.State.Raw = metricMetaReplacer.Replace(row.State.Raw)
	for k, v := range row.Tags {
		if strings.HasPrefix(k, "rm:") {
			delete(row.Tags, k)
			continue
		}
		row.Tags[k] = metricMetaReplacer.Replace(v)
	}
}

func sanitizeBGPRow(row *ddsnmp.BGPRow) {
	row.RowKey = metricMetaReplacer.Replace(row.RowKey)
	row.Identity.RoutingInstance = metricMetaReplacer.Replace(row.Identity.RoutingInstance)
	row.Identity.Neighbor = metricMetaReplacer.Replace(row.Identity.Neighbor)
	row.Identity.RemoteAS = metricMetaReplacer.Replace(row.Identity.RemoteAS)
	row.Descriptors.LocalAddress = metricMetaReplacer.Replace(row.Descriptors.LocalAddress)
	row.Descriptors.LocalAS = metricMetaReplacer.Replace(row.Descriptors.LocalAS)
	row.Descriptors.LocalIdentifier = metricMetaReplacer.Replace(row.Descriptors.LocalIdentifier)
	row.Descriptors.PeerIdentifier = metricMetaReplacer.Replace(row.Descriptors.PeerIdentifier)
	row.Descriptors.PeerType = metricMetaReplacer.Replace(row.Descriptors.PeerType)
	row.Descriptors.BGPVersion = metricMetaReplacer.Replace(row.Descriptors.BGPVersion)
	row.Descriptors.Description = metricMetaReplacer.Replace(row.Descriptors.Description)
	sanitizeBGPState(&row.State)
	sanitizeBGPState(&row.Previous)
	sanitizeBGPBool(&row.Admin.Enabled)
	sanitizeBGPInt64(&row.Connection.EstablishedUptime)
	sanitizeBGPInt64(&row.Connection.LastReceivedUpdateAge)
	sanitizeBGPDirectional(&row.Traffic.Messages)
	sanitizeBGPDirectional(&row.Traffic.Updates)
	sanitizeBGPDirectional(&row.Traffic.Notifications)
	sanitizeBGPDirectional(&row.Traffic.RouteRefreshes)
	sanitizeBGPDirectional(&row.Traffic.Opens)
	sanitizeBGPDirectional(&row.Traffic.Keepalives)
	sanitizeBGPInt64(&row.Transitions.Established)
	sanitizeBGPInt64(&row.Transitions.Down)
	sanitizeBGPInt64(&row.Transitions.Up)
	sanitizeBGPInt64(&row.Transitions.Flaps)
	sanitizeBGPTimerPair(&row.Timers.Negotiated)
	sanitizeBGPTimerPair(&row.Timers.Configured)
	sanitizeBGPInt64(&row.LastError.Code)
	sanitizeBGPInt64(&row.LastError.Subcode)
	sanitizeBGPLastNotification(&row.LastNotify.Received)
	sanitizeBGPLastNotification(&row.LastNotify.Sent)
	sanitizeBGPText(&row.Reasons.LastDown)
	sanitizeBGPText(&row.Reasons.Unavailability)
	sanitizeBGPText(&row.Restart.State)
	sanitizeBGPRouteCounters(&row.Routes.Current)
	sanitizeBGPRouteCounters(&row.Routes.Total)
	sanitizeBGPInt64(&row.RouteLimits.Limit)
	sanitizeBGPInt64(&row.RouteLimits.Threshold)
	sanitizeBGPInt64(&row.RouteLimits.ClearThreshold)
	sanitizeBGPInt64(&row.Device.Peers)
	sanitizeBGPInt64(&row.Device.InternalPeers)
	sanitizeBGPInt64(&row.Device.ExternalPeers)
	for k, v := range row.Tags {
		if strings.HasPrefix(k, "rm:") {
			delete(row.Tags, k)
			continue
		}
		row.Tags[k] = metricMetaReplacer.Replace(v)
	}
}

func sanitizeBGPState(value *ddsnmp.BGPState) {
	value.Raw = metricMetaReplacer.Replace(value.Raw)
}

func sanitizeBGPInt64(value *ddsnmp.BGPInt64) {
	value.Raw = metricMetaReplacer.Replace(value.Raw)
}

func sanitizeBGPText(value *ddsnmp.BGPText) {
	value.Raw = metricMetaReplacer.Replace(value.Raw)
	value.Value = metricMetaReplacer.Replace(value.Value)
}

func sanitizeBGPBool(value *ddsnmp.BGPBool) {
	value.Raw = metricMetaReplacer.Replace(value.Raw)
}

func sanitizeBGPDirectional(value *ddsnmp.BGPDirectional) {
	sanitizeBGPInt64(&value.Received)
	sanitizeBGPInt64(&value.Sent)
}

func sanitizeBGPTimerPair(value *ddsnmp.BGPTimerPair) {
	sanitizeBGPInt64(&value.ConnectRetry)
	sanitizeBGPInt64(&value.HoldTime)
	sanitizeBGPInt64(&value.KeepaliveTime)
	sanitizeBGPInt64(&value.MinASOriginationInterval)
	sanitizeBGPInt64(&value.MinRouteAdvertisementInterval)
}

func sanitizeBGPLastNotification(value *ddsnmp.BGPLastNotification) {
	sanitizeBGPInt64(&value.Code)
	sanitizeBGPInt64(&value.Subcode)
	sanitizeBGPText(&value.Reason)
}

func sanitizeBGPRouteCounters(value *ddsnmp.BGPRouteCounters) {
	sanitizeBGPInt64(&value.Received)
	sanitizeBGPInt64(&value.Accepted)
	sanitizeBGPInt64(&value.Rejected)
	sanitizeBGPInt64(&value.Active)
	sanitizeBGPInt64(&value.Advertised)
	sanitizeBGPInt64(&value.Suppressed)
	sanitizeBGPInt64(&value.Withdrawn)
}

var metricMetaReplacer = strings.NewReplacer(
	"'", "",
	"\n", " ",
	"\r", " ",
	"\x00", "",
)
