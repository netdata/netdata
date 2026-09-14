// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExistingProfiles_MetricPresentation(t *testing.T) {
	catalog, err := cwprofiles.DefaultCatalog()
	require.NoError(t, err)

	tests := map[string]struct {
		profile    string
		metricName string
		statistics []string
		rate       bool
	}{
		"RDS disk queue depth": {
			profile: "rds", metricName: "DiskQueueDepth", statistics: []string{"average"},
		},
		"RDS network receive throughput": {
			profile: "rds", metricName: "NetworkReceiveThroughput", statistics: []string{"average"},
		},
		"RDS network transmit throughput": {
			profile: "rds", metricName: "NetworkTransmitThroughput", statistics: []string{"average"},
		},
		"RDS replica lag": {
			profile: "rds", metricName: "ReplicaLag", statistics: []string{"maximum"},
		},
		"RDS swap usage": {
			profile: "rds", metricName: "SwapUsage", statistics: []string{"average"},
		},
		"SQS oldest message age": {
			profile: "sqs", metricName: "ApproximateAgeOfOldestMessage", statistics: []string{"maximum"},
		},
		"SQS messages sent": {
			profile: "sqs", metricName: "NumberOfMessagesSent", statistics: []string{"sum"}, rate: true,
		},
		"SQS messages received": {
			profile: "sqs", metricName: "NumberOfMessagesReceived", statistics: []string{"sum"}, rate: true,
		},
		"SQS messages deleted": {
			profile: "sqs", metricName: "NumberOfMessagesDeleted", statistics: []string{"sum"}, rate: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile, ok := catalog.Get(tc.profile)
			require.True(t, ok)
			for _, metric := range profile.Metrics {
				if metric.MetricName != tc.metricName {
					continue
				}
				assert.Equal(t, tc.statistics, metric.Statistics)
				assert.Equal(t, tc.rate, metric.Rate)
				return
			}
			require.Failf(t, "metric not found", "profile=%q MetricName=%q", tc.profile, tc.metricName)
		})
	}
}

func TestExistingProfiles_ChartPresentation(t *testing.T) {
	catalog, err := cwprofiles.DefaultCatalog()
	require.NoError(t, err)

	tests := map[string]struct {
		profile    string
		context    string
		units      string
		selectors  []string
		dimensions []string
	}{
		"RDS disk queue depth": {
			profile: "rds", context: "disk_queue_depth", units: "operations",
			selectors: []string{"rds.disk_queue_depth_average"}, dimensions: []string{"queued"},
		},
		"RDS network throughput": {
			profile: "rds", context: "network_throughput", units: "bytes/s",
			selectors:  []string{"rds.network_receive_throughput_average", "rds.network_transmit_throughput_average"},
			dimensions: []string{"received", "sent"},
		},
		"RDS swap usage": {
			profile: "rds", context: "swap_usage", units: "bytes",
			selectors: []string{"rds.swap_usage_average"}, dimensions: []string{"swap"},
		},
		"SQS oldest message age": {
			profile: "sqs", context: "oldest_message_age", units: "seconds",
			selectors: []string{"sqs.approximate_age_of_oldest_message_maximum"}, dimensions: []string{"age"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile, ok := catalog.Get(tc.profile)
			require.True(t, ok)
			for _, chart := range profile.Template.Charts {
				if chart.Context != tc.context {
					continue
				}
				assert.Equal(t, tc.units, chart.Units)
				assert.Equal(t, "absolute", chart.Algorithm)
				require.Len(t, chart.Dimensions, len(tc.selectors))
				for i, dimension := range chart.Dimensions {
					assert.Equal(t, tc.selectors[i], dimension.Selector)
					assert.Equal(t, tc.dimensions[i], dimension.Name)
				}
				return
			}
			require.Failf(t, "chart not found", "profile=%q context=%q", tc.profile, tc.context)
		})
	}
}
