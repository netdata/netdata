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
	c.reportDeviceLifecycle(func(store deviceLifecycleStore) {
		store.RegisterJob(c.deviceStoreOwnerKey(), ddsnmp.DeviceLifecycleInfo{
			Hostname:    c.Hostname,
			Port:        c.Options.Port,
			SNMPVersion: c.Options.Version,
		})
	})
}

func (c *Collector) completeDeviceLifecycle(phase ddsnmp.DeviceLifecyclePhase, err error) {
	outcome := ddsnmp.DeviceLifecycleOutcomeSuccess
	if err != nil {
		outcome = ddsnmp.DeviceLifecycleOutcomeFailed
	}
	c.reportDeviceLifecycle(func(store deviceLifecycleStore) {
		store.RecordJobLifecycle(c.deviceStoreOwnerKey(), ddsnmp.DeviceLifecycleStatus{
			Phase:       phase,
			Outcome:     outcome,
			CompletedAt: time.Now(),
		})
	})
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
