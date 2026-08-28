// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

const deviceLifecycleWarningEvery = time.Hour

type deviceLifecycleStore interface {
	RegisterJob(string, ddsnmp.DeviceLifecycleInfo)
	RecordJobLifecycle(string, ddsnmp.DeviceLifecycleStatus)
}

func (c *Collector) beginDeviceLifecycle() {
	info := ddsnmp.DeviceLifecycleInfo{
		Hostname:    c.Hostname,
		Port:        c.Options.Port,
		SNMPVersion: c.Options.Version,
	}
	c.deviceLifecycleMu.Lock()
	c.deviceLifecycleInfo = info
	publish := !c.deviceLifecycleManaged || c.deviceLifecycleCommitted
	c.deviceLifecycleMu.Unlock()
	if !publish {
		return
	}
	c.reportDeviceLifecycle(func(store deviceLifecycleStore) {
		store.RegisterJob(c.deviceStoreOwnerKey(), info)
	})
}

func (c *Collector) completeDeviceLifecycle(phase ddsnmp.DeviceLifecyclePhase, err error) {
	outcome := ddsnmp.DeviceLifecycleOutcomeSuccess
	if err != nil {
		outcome = ddsnmp.DeviceLifecycleOutcomeFailed
	}
	c.recordDeviceLifecycle(phase, outcome)
}

func (c *Collector) recordDeviceLifecycle(
	phase ddsnmp.DeviceLifecyclePhase,
	outcome ddsnmp.DeviceLifecycleOutcome,
) {
	status := ddsnmp.DeviceLifecycleStatus{
		Phase:       phase,
		Outcome:     outcome,
		CompletedAt: time.Now(),
	}
	c.deviceLifecycleMu.Lock()
	c.deviceLifecycleStatus = status
	publish := !c.deviceLifecycleManaged || c.deviceLifecycleCommitted
	c.deviceLifecycleMu.Unlock()
	if !publish {
		return
	}
	c.reportDeviceLifecycle(func(store deviceLifecycleStore) {
		store.RecordJobLifecycle(c.deviceStoreOwnerKey(), status)
	})
}

func (c *Collector) bindDeviceLifecycle(owner string) {
	if c == nil || owner == "" {
		return
	}
	c.deviceLifecycleMu.Lock()
	defer c.deviceLifecycleMu.Unlock()
	if c.deviceLifecycleOwner != "" && c.deviceLifecycleOwner != owner {
		return
	}
	c.deviceLifecycleOwner = owner
	c.deviceLifecycleManaged = true
	c.deviceLifecycleCommitted = false
}

func (c *Collector) commitDeviceLifecycle(previousOwner string) {
	if c == nil || c.deviceStore == nil {
		return
	}
	c.deviceLifecycleMu.Lock()
	defer c.deviceLifecycleMu.Unlock()
	defer func() {
		if recover() != nil {
			c.Limit("snmp:device-lifecycle", 1, deviceLifecycleWarningEvery).
				Warningf("failed to update SNMP job lifecycle diagnostics")
		}
	}()
	c.deviceStore.ReplaceJob(
		previousOwner,
		c.deviceLifecycleOwner,
		c.deviceLifecycleInfo,
		c.deviceLifecycleStatus,
		c.deviceLifecyclePending,
	)
	c.deviceLifecyclePending = nil
	c.deviceLifecycleCommitted = true
}

func (c *Collector) deviceLifecycleBoundTo(owner string) bool {
	if c == nil || owner == "" {
		return false
	}
	c.deviceLifecycleMu.Lock()
	defer c.deviceLifecycleMu.Unlock()
	return c.deviceLifecycleManaged && c.deviceLifecycleOwner == owner
}

func (c *Collector) reportDeviceLifecycle(report func(deviceLifecycleStore)) {
	if c == nil || c.deviceLifecycleStore == nil {
		return
	}
	defer func() {
		if recover() != nil {
			c.Limit("snmp:device-lifecycle", 1, deviceLifecycleWarningEvery).
				Warningf("failed to update SNMP job lifecycle diagnostics")
		}
	}()
	report(c.deviceLifecycleStore)
}
