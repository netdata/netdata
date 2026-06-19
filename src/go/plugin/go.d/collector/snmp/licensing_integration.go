// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

type licensingIntegration struct {
	cache *licenseCache
}

func newLicensingIntegration() *licensingIntegration {
	return &licensingIntegration{cache: newLicenseCache()}
}

func (li *licensingIntegration) registerFunction(r *funcRouter) {
	if li == nil || r == nil {
		return
	}
	r.registerHandler(licensesMethodID, newFuncLicenses(li.cache))
}

func snmpMethods() []funcapi.MethodConfig {
	methods := snmpBaseMethods()
	return append(methods, licensesMethodConfig())
}

func (c *Collector) collectLicensing(mx map[string]int64, pms []*ddsnmp.ProfileMetrics) {
	if c.licensing == nil {
		return
	}
	c.licensing.collect(c, mx, pms)
}

func (li *licensingIntegration) collect(c *Collector, mx map[string]int64, pms []*ddsnmp.ProfileMetrics) {
	now := time.Now().UTC()
	rows := extractLicenseRows(pms, now)

	if li.cache != nil {
		li.cache.store(now, rows)
	}

	if len(rows) == 0 {
		return
	}

	agg := aggregateLicenseRows(rows, now)
	if !agg.empty() {
		c.addLicenseCharts(agg)
	}
	agg.writeTo(mx)
}
