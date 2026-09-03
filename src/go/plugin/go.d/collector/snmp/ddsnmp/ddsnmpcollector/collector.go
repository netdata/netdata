// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type Config struct {
	SnmpClient                 gosnmp.Handler
	Profiles                   []*ddsnmp.Profile
	Log                        *logger.Logger
	SysObjectID                string
	DisableBulkWalk            bool
	InitialAcquisitionObserver AcquisitionObserver
}

func New(cfg Config) *Collector {
	coll := &Collector{
		log:                       cfg.Log.With(slog.String("ddsnmp", "collector")),
		profiles:                  make(map[string]*profileState),
		missingOIDs:               make(map[string]bool),
		regularScalarNamesScratch: make(map[string]struct{}),
		tableCache:                newTableCache(30*time.Minute, 1),
		tableIdentity:             buildTableIdentity(cfg.Profiles),
	}

	for _, prof := range cfg.Profiles {
		coll.profiles[prof.SourceFile] = &profileState{profile: prof}
	}
	if cfg.InitialAcquisitionObserver != nil {
		coll.initialAcquisitionObserver = cfg.InitialAcquisitionObserver
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
		log                        *logger.Logger
		profiles                   map[string]*profileState
		missingOIDs                map[string]bool
		regularScalarNamesScratch  map[string]struct{}
		tableCache                 *tableCache
		tableIdentity              *tableIdentity
		initialAcquisitionObserver AcquisitionObserver

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
	preparedProfileCollection struct {
		state           *profileState
		metrics         *ddsnmp.ProfileMetrics
		acquisition     *acquisitionProfileCollection
		topologyProfile *ddsnmp.Profile
		topologyKinds   map[topologyMetricLookupKey]ddprofiledefinition.TopologyKind
		regularScope    *tableCollectionScope
		topologyScope   *tableCollectionScope
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
			ddsnmp.MergeMetaTag(meta, k, v)
		}
	}

	return meta, nil
}

func (c *Collector) Collect() ([]*ddsnmp.ProfileMetrics, error) {
	var prepared []*preparedProfileCollection
	var errs []error
	if c.initialAcquisitionObserver != nil {
		defer c.releaseInitialAcquisition()
	}

	expired := c.tableCache.clearExpired()
	if len(expired) > 0 {
		c.log.Debugf("Cleared %d expired table cache entries", len(expired))
	}

	for ordinal, state := range c.sortedProfileStates() {
		profile, err := c.prepareProfileCollection(state, uint32(ordinal))
		if err != nil {
			errs = append(errs, err)
			c.observeAcquisitionProfile(profile, AcquisitionProfileOutcomeFailed, AcquisitionFailurePhasePrepare)
			continue
		}
		prepared = append(prepared, profile)
	}

	session := newTableCollectionSession(c.tableCollector, c.tableIdentity)
	for _, profile := range prepared {
		profile.regularScope = session.addScope(
			profile.state.profile,
			tableSymbolModeValue,
			&profile.metrics.Stats,
		)
		profile.regularScope.execution = profile.acquisition.executionReport()
		if profile.topologyProfile != nil {
			profile.topologyScope = session.addObservedScope(
				profile.topologyProfile,
				tableSymbolModePresence,
				&profile.metrics.Stats,
				profile.acquisition.topologyTableScope(),
			)
			profile.topologyScope.execution = profile.acquisition.executionReport()
		}
	}
	session.resolve()

	var metrics []*ddsnmp.ProfileMetrics
	for _, profile := range prepared {
		if err := c.collectPreparedProfileTables(profile, session); err != nil {
			errs = append(errs, err)
			c.observeAcquisitionProfile(profile, AcquisitionProfileOutcomeFailed, AcquisitionFailurePhaseTables)
			continue
		}

		c.collectPreparedProfileRows(profile)
		c.attachProfileMetrics(profile.metrics)
		c.updateProfileMetrics(profile.metrics)
		metrics = append(metrics, profile.metrics)

		now := time.Now()
		vmetrics := c.vmetricsCollector.collect(profile.state.profile.Definition, profile.metrics.Metrics)
		profile.metrics.Stats.Timing.VirtualMetrics = time.Since(now)
		if len(vmetrics) > 0 {
			for i := range vmetrics {
				vmetrics[i].Profile = profile.metrics
			}

			profile.metrics.Metrics = append(profile.metrics.Metrics, vmetrics...)
			profile.metrics.Stats.Metrics.Virtual += int64(len(vmetrics))
		}

		profile.metrics.HiddenMetrics = collectHiddenMetrics(profile.metrics.Metrics)
		profile.metrics.Metrics = slices.DeleteFunc(profile.metrics.Metrics, func(m ddsnmp.Metric) bool {
			return strings.HasPrefix(m.Name, "_")
		})
		c.observeAcquisitionProfile(profile, AcquisitionProfileOutcomeSuccess, AcquisitionFailurePhaseNone)
	}

	if len(metrics) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	if len(errs) > 0 {
		c.log.Debugf("collecting metrics: %v", errors.Join(errs...))
	}

	return metrics, nil
}

func (c *Collector) observeAcquisitionProfile(
	profile *preparedProfileCollection,
	outcome AcquisitionProfileOutcome,
	phase AcquisitionFailurePhase,
) {
	if c.initialAcquisitionObserver == nil || profile == nil || profile.acquisition == nil {
		return
	}
	profile.acquisition.syncTableRoutes()
	report := profile.acquisition.report(outcome, phase, profile.metrics)
	defer func() {
		_ = recover()
	}()
	c.initialAcquisitionObserver.ObserveProfile(report, profile.metrics)
}

func (c *Collector) releaseInitialAcquisition() {
	c.initialAcquisitionObserver = nil
}

func (c *Collector) sortedProfileStates() []*profileState {
	keys := make([]string, 0, len(c.profiles))
	for source := range c.profiles {
		keys = append(keys, source)
	}
	sort.Strings(keys)

	profiles := make([]*profileState, 0, len(keys))
	for _, source := range keys {
		profiles = append(profiles, c.profiles[source])
	}
	return profiles
}

func (c *Collector) keepFirstRegularScalarMetricByName(metrics []ddsnmp.Metric) []ddsnmp.Metric {
	if len(metrics) < 2 {
		return metrics
	}
	if c.regularScalarNamesScratch == nil {
		c.regularScalarNamesScratch = make(map[string]struct{})
	} else {
		clear(c.regularScalarNamesScratch)
	}

	return slices.DeleteFunc(metrics, func(metric ddsnmp.Metric) bool {
		if _, ok := c.regularScalarNamesScratch[metric.Name]; ok {
			return true
		}
		c.regularScalarNamesScratch[metric.Name] = struct{}{}
		return false
	})
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

func (c *Collector) prepareProfileCollection(ps *profileState, ordinal uint32) (*preparedProfileCollection, error) {
	pm := &ddsnmp.ProfileMetrics{Source: ps.profile.SourceFile}
	prepared := &preparedProfileCollection{
		state:   ps,
		metrics: pm,
	}
	if c.initialAcquisitionObserver != nil {
		prepared.acquisition = newAcquisitionProfileCollection(
			buildAcquisitionProfileIdentity(ps.profile, ordinal),
			ps.profile,
			c.deviceMetadataCollector.sysobjectid,
		)
	}

	if err := c.prepareProfileInputs(prepared); err != nil {
		return prepared, err
	}

	now := time.Now()
	scalarMetrics, err := c.scalarCollector.collect(ps.profile, &pm.Stats)
	pm.Stats.Timing.Scalar = time.Since(now)
	if err != nil {
		return prepared, err
	}
	scalarMetrics = c.keepFirstRegularScalarMetricByName(scalarMetrics)
	pm.Metrics = append(pm.Metrics, scalarMetrics...)
	pm.Stats.Metrics.Scalar += int64(len(scalarMetrics))

	topologyProfile, topologyKinds := buildTopologyProfile(ps.profile)
	if topologyProfile != nil {
		var topologyScalars []ddsnmp.Metric
		now = time.Now()
		if prepared.acquisition == nil {
			topologyScalars, err = c.scalarCollector.collect(topologyProfile, &pm.Stats)
		} else {
			topologyScalars, err = c.scalarCollector.collectObserved(
				topologyProfile,
				&pm.Stats,
				prepared.acquisition.topologyScalarObserver(),
			)
		}
		pm.Stats.Timing.Scalar += time.Since(now)
		if err != nil {
			return prepared, err
		}
		assignTopologyKinds(topologyScalars, topologyKinds)
		pm.TopologyMetrics = append(pm.TopologyMetrics, topologyScalars...)
	}

	prepared.topologyProfile = topologyProfile
	prepared.topologyKinds = topologyKinds
	return prepared, nil
}

func (c *Collector) prepareProfileInputs(prepared *preparedProfileCollection) error {
	ps, pm := prepared.state, prepared.metrics
	started := time.Now()
	defer func() {
		pm.Stats.Timing.Preparation = time.Since(started)
		if execution := prepared.acquisition.executionReport(); execution != nil {
			// Snapshot the same counters before subsequent phases add their work.
			execution.Preparation = AcquisitionPreparationStats{
				Elapsed:          pm.Stats.Timing.Preparation,
				GetRequests:      pm.Stats.SNMP.GetRequests,
				GetOIDs:          pm.Stats.SNMP.GetOIDs,
				SNMPErrors:       pm.Stats.Errors.SNMP,
				MissingOIDs:      pm.Stats.Errors.MissingOIDs,
				ProcessingErrors: pm.Stats.Errors.Processing.Preparation,
			}
		}
	}()
	if !ps.initialized {
		globalTags, err := c.globalTagsCollector.collectObserved(ps.profile, &pm.Stats, prepared.acquisition)
		if err != nil {
			return fmt.Errorf("failed to collect global tags: %w", err)
		}
		ps.cache.globalTags = globalTags
		metadata, err := c.deviceMetadataCollector.collectObserved(ps.profile, &pm.Stats, prepared.acquisition)
		if err != nil {
			return fmt.Errorf("failed to collect device metadata: %w", err)
		}
		ps.cache.deviceMetadata = metadata
		ps.initialized = true
	}
	pm.Tags = maps.Clone(ps.cache.globalTags)
	pm.DeviceMetadata = maps.Clone(ps.cache.deviceMetadata)
	return nil
}

func (c *Collector) collectPreparedProfileTables(
	profile *preparedProfileCollection,
	session *tableCollectionSession,
) error {
	now := time.Now()
	tableMetrics, err := session.collectScope(profile.regularScope)
	profile.metrics.Stats.Timing.Table += time.Since(now)
	if err != nil {
		return err
	}
	profile.metrics.Metrics = append(profile.metrics.Metrics, tableMetrics...)
	profile.metrics.Stats.Metrics.Table += int64(len(tableMetrics))

	if profile.topologyScope == nil {
		return nil
	}
	now = time.Now()
	topologyMetrics, err := session.collectScope(profile.topologyScope)
	profile.metrics.Stats.Timing.Table += time.Since(now)
	if err != nil {
		return err
	}
	assignTopologyKinds(topologyMetrics, profile.topologyKinds)
	profile.metrics.TopologyMetrics = append(profile.metrics.TopologyMetrics, topologyMetrics...)
	return nil
}

func (c *Collector) collectPreparedProfileRows(profile *preparedProfileCollection) {
	ps := profile.state
	pm := profile.metrics

	now := time.Now()
	licenseRows, err := c.collectLicenseRowsObserved(ps.profile, &pm.Stats, profile.acquisition.executionReport())
	if err != nil {
		c.log.Limit(licenseRowsFailedLogKey+ps.profile.SourceFile, 1, licenseRowsErrorLogEvery).
			Warningf("failed to collect licensing rows for profile %q: %v", ps.profile.SourceFile, err)
	}
	pm.LicenseRows = append(pm.LicenseRows, licenseRows...)
	pm.Stats.Metrics.Licensing += int64(len(licenseRows))
	pm.Stats.Timing.Licensing = time.Since(now)

	now = time.Now()
	var bgpRows []ddsnmp.BGPRow
	if profile.acquisition == nil {
		bgpRows, err = c.collectBGPRows(ps.profile, &pm.Stats)
	} else {
		bgpRows, err = c.collectBGPRowsObserved(ps.profile, &pm.Stats, profile.acquisition)
	}
	if err != nil {
		pm.BGPCollectError = err
		c.log.Limit(bgpRowsFailedLogKey+ps.profile.SourceFile, 1, bgpRowsErrorLogEvery).
			Warningf("failed to collect BGP rows for profile %q: %v", ps.profile.SourceFile, err)
	}
	pm.BGPRows = append(pm.BGPRows, bgpRows...)
	pm.Stats.Metrics.BGP += int64(len(bgpRows))
	pm.Stats.Timing.BGP = time.Since(now)
}

func (c *Collector) attachProfileMetrics(pm *ddsnmp.ProfileMetrics) {
	for i := range pm.Metrics {
		pm.Metrics[i].Profile = pm
	}
	for i := range pm.TopologyMetrics {
		pm.TopologyMetrics[i].Profile = pm
	}
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
