// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import "time"

const recurringLogEvery = time.Hour

const (
	warningKeyDiscoveryCache = "discovery_cache"
	warningKeyMetrics        = "metrics"
	warningKeyBGP            = "bgp"
	warningKeyNormalization  = "normalization"
)

func (c *Collector) warnRecoverable(key, class, format string, args ...any) {
	c.Limit("cato:"+key+":"+class, 1, recurringLogEvery).Warningf(format, args...)
}

func (c *Collector) logNormalizationIssue(surface, issue string) {
	c.Limit("cato:"+warningKeyNormalization+":"+surface+":"+issue, 1, recurringLogEvery).
		Warningf("Cato API payload normalization issue, surface=%s issue=%s", surface, issue)
}
