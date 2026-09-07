// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
)

type topologyDiagnosticProvider struct {
	registry *topologyRegistry
	aborted  *atomic.Pointer[topologyAbortedSweepDiagnostic]
	source   deviceLifecycleSource
	limits   topologyAcquisitionLimits
}

func (p *topologyDiagnosticProvider) Capture() (snmpdiag.Snapshot, error) {
	diagnostics, limits := captureTopologyCut(p.registry, p.aborted.Load(), p.limits)
	diagnostics.lifecycle.state = diagnosticCaptureAvailable
	snapshot, err := newTopologyDiagnosticArchiveSnapshotV1(diagnostics)
	if err != nil {
		return snmpdiag.Snapshot{}, err
	}
	snapshot.Lifecycle = snmpdiag.CaptureLifecycle(p.source, limits.maxRecords, limits.maxLogicalBytes)
	return snapshot, nil
}

type topologyJobConfigLifecycle struct{ publisher *snmpdiag.Publisher }
type topologyConfigSnapshot struct {
	id collectorapi.JobConfigIdentity
}

func (s topologyConfigSnapshot) Identity() collectorapi.JobConfigIdentity { return s.id }
func (*topologyJobConfigLifecycle) Project(id collectorapi.JobConfigIdentity, _ map[string]any) collectorapi.JobConfigLifecycleSnapshot {
	return topologyConfigSnapshot{id}
}
func (*topologyJobConfigLifecycle) Bind(collectorapi.JobConfigIdentity, collectorapi.RuntimeJob) {}
func (*topologyJobConfigLifecycle) Capture(id collectorapi.JobConfigIdentity, _ collectorapi.RuntimeJob) collectorapi.JobConfigLifecycleSnapshot {
	return topologyConfigSnapshot{id}
}
func (h *topologyJobConfigLifecycle) Reconcile(_ collectorapi.JobConfigIdentity, snapshot collectorapi.JobConfigLifecycleSnapshot, job collectorapi.RuntimeJob) {
	if h.publisher == nil {
		return
	}
	if job != nil {
		if c, ok := job.Collector().(*Collector); ok {
			h.publisher.SetTopology(snapshot.Identity().String(), c.diagnosticProvider, max(c.deviceCheckEvery(), c.refreshEvery()))
			return
		}
	}
	h.publisher.SetTopology(snapshot.Identity().String(), nil, snmpdiag.DefaultInterval)
}
func (h *topologyJobConfigLifecycle) Remove(id collectorapi.JobConfigIdentity) {
	if h.publisher != nil {
		h.publisher.RemoveTopology(id.String())
	}
}

func (c *Collector) releaseDiagnosticProvider() {
	if c.diagnosticPublisher != nil {
		c.diagnosticPublisher.ReleaseTopology(c.diagnosticProvider)
	}
}
