// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_apiserver

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (c *Collector) initPrometheusClient() (prometheus.Prometheus, error) {
	httpClient, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return nil, err
	}

	if srParseErr != nil {
		c.Warningf("selector parse error (collecting all metrics): %v", srParseErr)
	}

	return prometheus.NewWithSelector(httpClient, c.RequestConfig, sr), nil
}

// Selector to filter only the metrics we need, reducing memory usage
// Parse() is called at package init time; if it fails, sr will be nil and all metrics will be collected
var sr, srParseErr = selector.Expr{
	Allow: []string{
		// Request metrics
		"apiserver_request_total",
		"apiserver_request_count",
		"apiserver_request_duration_seconds",
		"apiserver_response_sizes",
		"apiserver_dropped_requests_total",
		"apiserver_flowcontrol_rejected_requests_total",

		// Inflight metrics
		"apiserver_current_inflight_requests",
		"apiserver_longrunning_requests",

		// REST client metrics
		"rest_client_requests_total",
		"rest_client_request_duration_seconds",

		// Admission metrics
		"apiserver_admission_step_admission_duration_seconds",
		"apiserver_admission_controller_admission_duration_seconds",
		"apiserver_admission_webhook_admission_duration_seconds",

		// Etcd/storage metrics
		"apiserver_storage_objects",
		"etcd_object_counts",

		// Workqueue metrics
		"workqueue_depth",
		"workqueue_adds_total",
		"workqueue_retries_total",
		"workqueue_queue_duration_seconds",
		"workqueue_work_duration_seconds",

		// Audit metrics
		"apiserver_audit_event_total",
		"apiserver_audit_requests_rejected_total",

		// Auth metrics
		"authenticated_user_requests",
		"apiserver_client_certificate_expiration_seconds",

		// Process metrics
		"go_goroutines",
		"go_threads",
		"go_gc_duration_seconds",
		"go_memstats_heap_alloc_bytes",
		"go_memstats_heap_inuse_bytes",
		"go_memstats_stack_inuse_bytes",
		"process_cpu_seconds_total",
		"process_resident_memory_bytes",
		"process_virtual_memory_bytes",
		"process_open_fds",
		"process_max_fds",
	},
}.Parse()
