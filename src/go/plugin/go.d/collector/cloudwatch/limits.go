// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

// Fixed safety envelopes bound query expansion, AWS response work, and local
// discovery matching. The operator-controlled discovery-group safeguard is
// part of LimitsConfig.
const (
	maxDatapointsPerRequest                = 30000
	maxGetMetricDataPages                  = 2
	maxPlannedQueries                      = 20000
	maxPlannedDatapoints                   = 600000
	maxPlannedQueryBatches                 = 40
	maxDiscoveryGroupsPerJob               = 100
	maxListMetricsPages                    = 100
	maxScannedMetricsPerGroup              = 50000
	maxCandidateInstancesPerGroup          = 20000
	maxDiscoveryMatcherEvaluationsPerGroup = 1000000
)
