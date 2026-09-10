// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"maps"
	"slices"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// Mutable builders belong to the synchronous collector. Only terminal immutable
// cuts cross into the serial publisher, never a Collector or live cache pointer.
type normalDiagnostics struct {
	sequence       uint64
	current        *normalAttempt
	recorder       *ddsnmpcollector.SourceRecorder
	client         gosnmp.Handler
	initialization []*ddsnmp.SourceOperation
	latest, failed *normalAttempt
	bgpSources     map[string][]*ddsnmp.SourceOperation
	licenseSources []*ddsnmp.SourceOperation
	cut            *normalCut
}

type normalAttempt struct {
	bgpSources     map[string][]*ddsnmp.SourceOperation
	licenseSources []*ddsnmp.SourceOperation
	document       diagnostics.NormalAttempt
	sources        []*ddsnmp.SourceOperation
	caches         []ddsnmpcollector.AcquisitionCacheInput
}

type normalCut struct {
	document       diagnostics.NormalDevice
	profileContext *ddsnmp.ProfileContext
	initialization []*ddsnmp.SourceOperation
	latest, failed *normalAttempt
}

func (c *Collector) beginNormalAttempt(phase string) {
	n := c.normal
	n.sequence++
	n.current = &normalAttempt{document: diagnostics.NormalAttempt{ID: n.sequence, Phase: phase, StartedAt: time.Now().UTC()}}
	n.recorder = &ddsnmpcollector.SourceRecorder{ContextID: n.sequence}
	c.wrapNormalClient()
}

func (c *Collector) wrapNormalClient() {
	n := c.normal
	if n.recorder == nil || c.snmpClient == nil || n.client != nil {
		return
	}
	n.client = c.snmpClient
	c.snmpClient = n.recorder.Wrap(n.client)
	if collector, ok := c.ddSnmpColl.(interface{ SetSNMPClient(gosnmp.Handler) }); ok {
		collector.SetSNMPClient(c.snmpClient)
	}
}

func (c *Collector) observeNormalProfile(report ddsnmpcollector.AcquisitionProfileReport, pm *ddsnmp.ProfileMetrics) {
	if c.normal.current == nil {
		return
	}
	profile := diagnostics.NormalProfile{Acquisition: report}
	if pm != nil {
		profile.Source = pm.Source
		profile.Tags, profile.Metadata = maps.Clone(pm.Tags), maps.Clone(pm.DeviceMetadata)
		profile.Metrics, profile.HiddenMetrics = normalMetrics(pm.Metrics), normalMetrics(pm.HiddenMetrics)
		profile.BGPFailed = pm.BGPCollectError != nil
		profile.BGPRows = slices.Clone(pm.BGPRows)
		for i := range profile.BGPRows {
			profile.BGPRows[i].Tags = maps.Clone(profile.BGPRows[i].Tags)
			profile.BGPRows[i].Device.ByState = maps.Clone(profile.BGPRows[i].Device.ByState)
		}
		profile.LicenseRows = slices.Clone(pm.LicenseRows)
		for i := range profile.LicenseRows {
			profile.LicenseRows[i].Tags = maps.Clone(profile.LicenseRows[i].Tags)
		}
	}
	c.normal.current.document.Profiles = append(c.normal.current.document.Profiles, profile)
}

func (c *Collector) finishNormalAttempt(err error, samples map[string]int64) {
	n := c.normal
	if n.current == nil {
		return
	}
	attempt := n.current
	n.current = nil
	attempt.sources = n.recorder.Finish()
	n.recorder = nil
	if n.client != nil {
		c.snmpClient = n.client
		if collector, ok := c.ddSnmpColl.(interface{ SetSNMPClient(gosnmp.Handler) }); ok {
			collector.SetSNMPClient(n.client)
		}
		n.client = nil
	}
	attempt.document.CompletedAt = time.Now().UTC()
	attempt.document.Failure = snmputils.ClassifyFailure(err)
	attempt.document.Failed = err != nil || normalSourcesFailed(attempt.sources)
	attempt.document.Samples = maps.Clone(samples)
	c.deviceLifecycleMu.Lock()
	attempt.document.Failures = c.deviceCollectionFailures
	c.deviceLifecycleMu.Unlock()
	if collector, ok := c.ddSnmpColl.(interface {
		CachedInputs() []ddsnmpcollector.AcquisitionCacheInput
		NegativeCauses() []ddsnmpcollector.AcquisitionNegativeCause
	}); ok {
		attempt.caches = collector.CachedInputs()
		attempt.document.NegativeCauses = collector.NegativeCauses()
	}
	for _, profile := range attempt.document.Profiles {
		if profile.Acquisition.HasFailures() {
			attempt.document.Failed = true
		}
	}
	n.latest = attempt
	if attempt.document.Failed {
		n.failed = attempt
	}
	if attempt.document.Phase == "check" && err == nil {
		n.initialization = slices.Clone(attempt.sources)
	}
	cut := &normalCut{latest: n.latest, failed: n.failed, initialization: slices.Clone(n.initialization)}
	cut.document = diagnostics.NormalDevice{Hostname: c.Hostname, CapturedAt: attempt.document.CompletedAt, Device: c.normalDeviceInput()}
	attempt.document.BGP, attempt.bgpSources = c.captureNormalBGP()
	attempt.document.Licensing, attempt.licenseSources = c.captureNormalLicensing()
	c.deviceLifecycleMu.Lock()
	cut.profileContext = c.deviceLifecycleInfo.Profiles
	n.cut = cut
	writer := c.normalWriter
	c.deviceLifecycleMu.Unlock()
	writer.Update(cut)
}

func normalSourcesFailed(sources []*ddsnmp.SourceOperation) bool {
	for _, source := range sources {
		if source.Failure.Reason != "" {
			return true
		}
	}
	return false
}

func normalMetrics(metrics []ddsnmp.Metric) []diagnostics.NormalMetric {
	result := make([]diagnostics.NormalMetric, len(metrics))
	for i, metric := range metrics {
		result[i] = normalMetric(metric)
	}
	return result
}

func (c *Collector) normalDeviceInput() diagnostics.DeviceInput {
	result := diagnostics.DeviceInput{Hostname: c.Hostname}
	if si := c.sysInfo; si != nil {
		result.SysObjectID, result.SysName, result.SysDescr = si.SysObjectID, si.Name, si.Descr
		result.SysContact, result.SysLocation, result.Vendor, result.Model = si.Contact, si.Location, si.Vendor, si.Model
	}
	if vnode := c.deviceVnode(); vnode != nil {
		result.VnodeGUID, result.VnodeLabels = vnode.GUID, maps.Clone(vnode.Labels)
	}
	return result
}

func (c *Collector) recordNormalMetric(metric ddsnmp.Metric, action string, ids []string) {
	if c.normal != nil && c.normal.current != nil {
		slices.Sort(ids)
		c.normal.current.document.Metrics = append(c.normal.current.document.Metrics, diagnostics.NormalMetricDecision{Metric: normalMetric(metric), Action: action, SampleIDs: ids})
	}
}
