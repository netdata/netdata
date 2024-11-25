// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns_recursor

// https://doc.powerdns.com/recursor/metrics.html
// https://docs.powerdns.com/recursor/performance.html#recursor-caches

// PowerDNS Recursor documentation has no section about statistics objects,
// fortunately authoritative has.
// https://doc.powerdns.com/authoritative/http-api/statistics.html#objects
type (
	statisticMetrics []statisticMetric
	statisticMetric  struct {
		Name  string
		Type  string
		Value any
	}
)
