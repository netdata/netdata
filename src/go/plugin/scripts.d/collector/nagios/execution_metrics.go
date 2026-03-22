// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

type executionMetrics struct {
	durationSeconds float64
	cpuTotalSeconds float64
	maxRSSBytes     float64
}

func executionMetricsFromResult(result checkRunResult) executionMetrics {
	return executionMetrics{
		durationSeconds: result.Duration.Seconds(),
		cpuTotalSeconds: (result.Usage.User + result.Usage.System).Seconds(),
		maxRSSBytes:     float64(result.Usage.MaxRSSBytes),
	}
}
