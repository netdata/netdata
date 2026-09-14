// SPDX-License-Identifier: GPL-3.0-or-later

package cwprofiles

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

var wantDisabledStockMetrics = map[string][]string{
	"alb":                {"client_tls_negotiation_errors", "grpc_request_count", "http_redirect_count", "ipv6_request_count", "rule_evaluations"},
	"alb_target":         {"anomalous_host_count", "mitigated_host_count", "target_tls_negotiation_errors"},
	"api_gateway":        {"cache_hit_count", "cache_miss_count"},
	"auto_scaling":       {"group_in_service_capacity", "group_pending_capacity", "group_standby_capacity", "group_terminating_capacity", "group_total_capacity", "warm_pool_desired_capacity", "warm_pool_min_size", "warm_pool_pending_capacity", "warm_pool_terminating_capacity", "warm_pool_total_capacity", "warm_pool_warmed_capacity"},
	"bedrock":            {"cache_read_input_tokens", "cache_write_input_tokens", "estimated_tpm_quota_usage"},
	"cloudfront":         {"cache_hit_rate", "error_rate_401", "error_rate_403", "error_rate_404", "error_rate_502", "error_rate_503", "error_rate_504", "origin_latency"},
	"docdb":              {"disk_queue_depth", "documents_deleted", "documents_inserted", "swap_usage"},
	"dynamodb":           {"conditional_check_failed_requests", "time_to_live_deleted_item_count", "transaction_conflict"},
	"dynamodb_operation": {"fault_injection_errors"},
	"ebs":                {"volume_consumed_read_write_ops", "volume_throughput_percentage", "volume_total_read_time", "volume_total_write_time"},
	"ec2":                {"cpu_credit_balance", "cpu_credit_usage", "disk_read_bytes", "disk_write_bytes", "ebs_read_bytes", "ebs_read_ops", "ebs_write_bytes", "ebs_write_ops", "network_packets_in", "network_packets_out", "status_check_failed_instance", "status_check_failed_system"},
	"efs":                {"metadata_read_io_bytes", "metadata_write_io_bytes"},
	"eks":                {"admission_webhook_requests_admit", "admission_webhook_requests_validating", "apiserver_flowcontrol_seats", "apiserver_latency_delete_p99", "apiserver_latency_patch_p99", "apiserver_latency_post_p99", "apiserver_latency_put_p99", "apiserver_requests_429", "scheduler_pending_pods_active", "scheduler_pending_pods_backoff", "scheduler_pending_pods_gated", "scheduler_pending_pods_unschedulable"},
	"elasticache":        {"curr_items", "replication_bytes", "replication_lag", "swap_usage"},
	"elb":                {"desync_mitigation_noncompliant", "surge_queue_length"},
	"eventbridge":        {"dead_letter_invocations", "invocation_attempts", "invocations_failed_to_be_sent_to_dlq", "invocations_sent_to_dlq", "retry_invocation_attempts", "successful_invocation_attempts", "throttled_rules"},
	"firehose":           {"bytes_per_second_limit", "records_per_second_limit"},
	"kinesis":            {"put_record_bytes", "put_records_bytes"},
	"lambda":             {"concurrent_executions", "dead_letter_errors", "iterator_age"},
	"msk":                {"cpu_io_wait"},
	"nat_gateway":        {"packets_in_from_destination", "packets_in_from_source", "packets_out_to_destination", "packets_out_to_source", "peak_bytes_per_second", "peak_packets_per_second"},
	"nlb":                {"client_tls_negotiation_errors", "port_allocation_errors"},
	"opensearch":         {"deleted_documents", "searchable_documents", "shards_active", "shards_unassigned"},
	"rds":                {"burst_balance", "cpu_credit_balance", "cpu_credit_usage"},
	"redshift":           {"concurrency_scaling_seconds", "read_latency", "total_table_count", "write_latency"},
	"s3_requests":        {"list_requests", "post_requests", "select_requests"},
	"sqs":                {"approximate_number_of_groups_with_inflight_messages", "number_of_deduplicated_sent_messages"},
	"step_functions":     {"executions_redriven", "redriven_executions_failed", "redriven_executions_succeeded"},
}

// TestStockProfiles_OptInInventory pins the stock cost boundary. A missing
// disabled flag would silently add a default GetMetricData query.
func TestStockProfiles_OptInInventory(t *testing.T) {
	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)

	gotDisabled := make(map[string][]string)
	var metrics, defaultMetrics, disabledMetrics, series, defaultSeries, disabledSeries, charts int
	profiles := catalog.AllProfiles()
	for _, rp := range profiles {
		metrics += len(rp.Config.Metrics)
		charts += len(chartIDs(rp.Config.Template))
		for _, metric := range rp.Config.Metrics {
			series += len(metric.Statistics)
			if metric.Disabled {
				disabledMetrics++
				disabledSeries += len(metric.Statistics)
				gotDisabled[rp.Name] = append(gotDisabled[rp.Name], metric.ID)
				continue
			}
			defaultMetrics++
			defaultSeries += len(metric.Statistics)
		}
		sort.Strings(gotDisabled[rp.Name])
	}

	assert.Equal(t, wantDisabledStockMetrics, gotDisabled)
	assert.Equal(t, 47, len(profiles))
	assert.Equal(t, 442, metrics)
	assert.Equal(t, 324, defaultMetrics)
	assert.Equal(t, 118, disabledMetrics)
	assert.Equal(t, 466, series)
	assert.Equal(t, 347, defaultSeries)
	assert.Equal(t, 119, disabledSeries)
	assert.Equal(t, 294, charts)
}

// TestStockProfiles_MetricChartCoverage guards every stock metric, including
// opt-in metrics. Every declared series must have a chart dimension, and every
// chart dimension must resolve to a declared series.
func TestStockProfiles_MetricChartCoverage(t *testing.T) {
	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)

	disabled := 0
	// Two-way coverage between declared metric series and chart selectors:
	//   (1) every chart selector resolves to a declared metric series (no dangling
	//       selector); Normalize leaves an unresolved shorthand un-qualified, so it
	//       will not be a key in the visible (fully-qualified) series set; and
	//   (2) every declared metric series is targeted by at least one chart dimension.
	//       An un-charted metric would still bill GetMetricData yet render nothing.
	for _, rp := range catalog.AllProfiles() {
		for _, metric := range rp.Config.Metrics {
			if metric.Disabled {
				disabled++
			}
		}
		visible := visibleSeriesForProfile(rp.Name, rp.Config.Metrics)
		selected := make(map[string]struct{})
		for _, sel := range chartSelectors(rp.Config.Template) {
			series, _, ok := splitSelectorSeries(sel)
			if !assert.Truef(t, ok, "%s: unparseable chart selector %q", rp.Name, sel) {
				continue
			}
			selected[series] = struct{}{}
			_, found := visible[series]
			assert.Truef(t, found, "%s: chart selector %q does not resolve to a declared metric series", rp.Name, sel)
		}
		for series := range visible {
			_, charted := selected[series]
			assert.Truef(t, charted, "%s: metric series %q has no chart dimension (would bill GetMetricData but render nothing)", rp.Name, series)
		}
	}
	assert.Positive(t, disabled, "stock catalog must contain opt-in metrics")
}

// chartSelectors returns every chart-dimension selector in a group, recursively.
func chartSelectors(group charttpl.Group) []string {
	var out []string
	for _, c := range group.Charts {
		for _, d := range c.Dimensions {
			out = append(out, d.Selector)
		}
	}
	for _, g := range group.Groups {
		out = append(out, chartSelectors(g)...)
	}
	return out
}
