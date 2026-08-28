// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

type snmpJobConfigLifecycle struct {
	store *ddsnmp.DeviceStore
}

func newSNMPJobConfigLifecycle(store *ddsnmp.DeviceStore) collectorapi.JobConfigLifecycle {
	return &snmpJobConfigLifecycle{store: store}
}

func (*snmpJobConfigLifecycle) Bind(identity collectorapi.JobConfigIdentity, job collectorapi.RuntimeJob) {
	if collector := snmpCollectorFromRuntimeJob(job); collector != nil {
		collector.bindDeviceLifecycle(identity.String())
	}
}

func (*snmpJobConfigLifecycle) Capture(
	identity collectorapi.JobConfigIdentity,
	job collectorapi.RuntimeJob,
) collectorapi.JobConfigLifecycleSnapshot {
	collector := snmpCollectorFromRuntimeJob(job)
	if collector == nil || !collector.deviceLifecycleBoundTo(identity.String()) {
		return nil
	}
	return &snmpJobConfigLifecycleSnapshot{identity: identity, collector: collector}
}

func (l *snmpJobConfigLifecycle) Remove(identity collectorapi.JobConfigIdentity) {
	if l != nil && l.store != nil {
		l.store.Unregister(identity.String())
	}
}

func snmpCollectorFromRuntimeJob(job collectorapi.RuntimeJob) *Collector {
	if job == nil {
		return nil
	}
	collector, _ := job.Collector().(*Collector)
	return collector
}

type snmpJobConfigLifecycleSnapshot struct {
	identity  collectorapi.JobConfigIdentity
	collector *Collector
}

func (s *snmpJobConfigLifecycleSnapshot) Identity() collectorapi.JobConfigIdentity {
	if s == nil {
		return collectorapi.JobConfigIdentity{}
	}
	return s.identity
}

func (s *snmpJobConfigLifecycleSnapshot) Commit(previous collectorapi.JobConfigIdentity) {
	if s != nil && s.collector != nil {
		s.collector.commitDeviceLifecycle(previous.String())
	}
}
