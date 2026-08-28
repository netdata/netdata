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

func (*snmpJobConfigLifecycle) Project(
	identity collectorapi.JobConfigIdentity,
	config map[string]any,
) collectorapi.JobConfigLifecycleSnapshot {
	if !identity.Valid() {
		return nil
	}
	defaults := defaultConfig()
	info := ddsnmp.DeviceLifecycleInfo{
		Hostname:    stringConfigValue(config, "hostname", defaults.Hostname),
		Port:        defaults.Options.Port,
		SNMPVersion: defaults.Options.Version,
	}
	if options := nestedConfigMap(config["options"]); options != nil {
		info.Port = intConfigValue(options, "port", info.Port)
		info.SNMPVersion = stringConfigValue(options, "version", info.SNMPVersion)
	}
	return &snmpJobConfigLifecycleSnapshot{identity: identity, info: info}
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
	collector.deviceLifecycleMu.Lock()
	snapshot := &snmpJobConfigLifecycleSnapshot{
		identity: identity,
		info:     collector.deviceLifecycleInfo,
		status:   collector.deviceLifecycleStatus,
	}
	collector.deviceLifecycleMu.Unlock()
	return snapshot
}

func (l *snmpJobConfigLifecycle) Reconcile(
	previous collectorapi.JobConfigIdentity,
	snapshot collectorapi.JobConfigLifecycleSnapshot,
	job collectorapi.RuntimeJob,
) {
	current, ok := snmpLifecycleSnapshot(snapshot)
	if !ok || l == nil || l.store == nil {
		return
	}
	collector := snmpCollectorFromRuntimeJob(job)
	if collector == nil || !collector.deviceLifecycleBoundTo(current.identity.String()) {
		l.store.ReplaceJob(previous.String(), current.identity.String(), current.info, current.status, nil)
		return
	}
	collector.commitDeviceLifecycle(previous.String())
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
	identity collectorapi.JobConfigIdentity
	info     ddsnmp.DeviceLifecycleInfo
	status   ddsnmp.DeviceLifecycleStatus
}

func (s *snmpJobConfigLifecycleSnapshot) Identity() collectorapi.JobConfigIdentity {
	if s == nil {
		return collectorapi.JobConfigIdentity{}
	}
	return s.identity
}

func snmpLifecycleSnapshot(
	snapshot collectorapi.JobConfigLifecycleSnapshot,
) (*snmpJobConfigLifecycleSnapshot, bool) {
	current, ok := snapshot.(*snmpJobConfigLifecycleSnapshot)
	return current, ok && current != nil && current.identity.Valid()
}

func nestedConfigMap(value any) map[string]any {
	switch value := value.(type) {
	case map[string]any:
		return value
	case map[any]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			if key, ok := key.(string); ok {
				result[key] = item
			}
		}
		return result
	default:
		return nil
	}
}

func stringConfigValue(config map[string]any, key string, fallback string) string {
	if value, ok := config[key].(string); ok {
		return value
	}
	return fallback
}

func intConfigValue(config map[string]any, key string, fallback int) int {
	switch value := config[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case uint:
		return int(value)
	case uint64:
		return int(value)
	case float64:
		return int(value)
	default:
		return fallback
	}
}
