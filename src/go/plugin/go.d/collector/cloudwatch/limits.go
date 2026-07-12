// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

// Fixed safety envelopes bound configuration expansion and AWS response work.
// They are collector invariants rather than operator tuning knobs.
const (
	maxDiscoveryGroups            = 64
	maxQueryBuckets               = 1440
	maxDatapointsPerRequest       = 30000
	maxGetMetricDataPages         = 2
	maxPlannedQueries             = 20000
	maxPlannedDatapoints          = 600000
	maxPlannedQueryBatches        = 40
	maxListMetricsPages           = 100
	maxScannedMetricsPerGroup     = 50000
	maxCandidateInstancesPerGroup = 20000
)
