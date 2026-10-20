// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sync"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
)

type topologyDiagnosticProvider struct {
	registry *topologyRegistry
	aborted  *atomic.Pointer[topologydiag.AbortedSweep]
	source   deviceLifecycleSource
	mu       sync.Mutex
	sequence uint64
	history  []*topologyCheckpoint
}

type topologyCheckpoint struct {
	id  uint64
	cut topologydiag.Cut
}

func (c *topologyCheckpoint) ID() uint64 { return c.id }
func (c *topologyCheckpoint) Capture() (snmpdiag.Snapshot, error) {
	return topologydiag.NewSnapshot(c.cut)
}

func (p *topologyDiagnosticProvider) Checkpoints() []snmpdiag.Checkpoint {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([]snmpdiag.Checkpoint, len(p.history))
	for i, checkpoint := range p.history {
		result[i] = checkpoint
	}
	return result
}

func (c *Collector) recordDiagnosticCheckpoint() {
	if c.diagnosticProvider == nil {
		return
	}
	p := c.diagnosticProvider
	cut := captureTopologyCut(p.registry, p.aborted.Load())
	p.mu.Lock()
	if cut.Topology == nil && cut.LastAborted == nil ||
		len(p.history) > 0 && sameTopologyCheckpoint(p.history[len(p.history)-1].cut, cut) {
		p.mu.Unlock()
		return
	}
	cut.Lifecycle = captureCheckpointLifecycle(p.source)
	p.sequence++
	if len(p.history) == snmpdiag.CheckpointRetention {
		copy(p.history, p.history[1:])
		p.history = p.history[:len(p.history)-1]
	}
	p.history = append(p.history, &topologyCheckpoint{id: p.sequence, cut: cut})
	p.mu.Unlock()
	if c.diagnosticPublisher != nil {
		c.diagnosticPublisher.TopologyUpdated(p)
	}
}

func captureCheckpointLifecycle(source deviceLifecycleSource) (result topologydiag.LifecycleCut) {
	result = topologydiag.LifecycleCut{State: topologydiag.CaptureUnavailable, Reason: topologydiag.CaptureReasonProjectionError}
	defer func() {
		if recover() != nil {
			result = topologydiag.LifecycleCut{State: topologydiag.CaptureUnavailable, Reason: topologydiag.CaptureReasonProjectionPanic}
		}
	}()
	if source != nil {
		result = topologydiag.LifecycleCut{State: topologydiag.CaptureAvailable, Cut: source.LifecycleCut()}
	}
	return result
}

// Compare only the fixed-size state and immutable evidence identities. Sweep
// clocks, selection bookkeeping and prior removal annotations are not new evidence.
func sameTopologyCheckpoint(a, b topologydiag.Cut) bool {
	if a.ProducerScopeID != b.ProducerScopeID || a.LastAborted != b.LastAborted {
		return false
	}
	if a.Topology == nil || b.Topology == nil {
		return a.Topology == b.Topology
	}
	x, y := a.Topology, b.Topology
	if x.CaptureState != y.CaptureState || x.CaptureReason != y.CaptureReason || len(x.Devices) != len(y.Devices) {
		return false
	}
	for i, left := range x.Devices {
		right := y.Devices[i]
		left.Selected, right.Selected = false, false
		if left != right {
			return false
		}
	}
	return true
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
			h.publisher.SetTopology(snapshot.Identity().String(), c.diagnosticProvider)
			return
		}
	}
	h.publisher.SetTopology(snapshot.Identity().String(), nil)
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
