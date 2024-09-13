// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubeproxy

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

type (
	// Charts is an alias for module.Charts
	Charts = module.Charts
	// Dims is an alias for module.Dims
	Dims = module.Dims
	// Dim is an alias for module.Dim
	Dim = module.Dim
)

var charts = Charts{
	{
		ID:    "kubeproxy_sync_proxy_rules",
		Title: "Sync Proxy Rules",
		Units: "events/s",
		Fam:   "sync proxy rules",
		Ctx:   "k8s_kubeproxy.kubeproxy_sync_proxy_rules",
		Dims: Dims{
			{ID: "sync_proxy_rules_count", Name: "sync proxy rules", Algo: module.Incremental},
		},
	},
	{
		ID:    "kubeproxy_sync_proxy_rules_latency",
		Title: "Sync Proxy Rules Latency",
		Units: "observes/s",
		Fam:   "sync proxy rules",
		Ctx:   "k8s_kubeproxy.kubeproxy_sync_proxy_rules_latency_microseconds",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "sync_proxy_rules_bucket_1000", Name: "0.001 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_2000", Name: "0.002 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_4000", Name: "0.004 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_8000", Name: "0.008 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_16000", Name: "0.016 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_32000", Name: "0.032 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_64000", Name: "0.064 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_128000", Name: "0.128 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_256000", Name: "0.256 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_512000", Name: "0.512 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_1024000", Name: "1.024 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_2048000", Name: "2.048 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_4096000", Name: "4.096 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_8192000", Name: "8.192 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_16384000", Name: "16.384 sec", Algo: module.Incremental},
			{ID: "sync_proxy_rules_bucket_+Inf", Name: "+Inf", Algo: module.Incremental},
		},
	},
	{
		ID:    "kubeproxy_sync_proxy_rules_latency_percentage",
		Title: "Sync Proxy Rules Latency Percentage",
		Units: "%",
		Fam:   "sync proxy rules",
		Ctx:   "k8s_kubeproxy.kubeproxy_sync_proxy_rules_latency",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "sync_proxy_rules_bucket_1000", Name: "0.001 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_2000", Name: "0.002 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_4000", Name: "0.004 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_8000", Name: "0.008 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_16000", Name: "0.016 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_32000", Name: "0.032 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_64000", Name: "0.064 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_128000", Name: "0.128 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_256000", Name: "0.256 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_512000", Name: "0.512 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_1024000", Name: "1.024 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_2048000", Name: "2.048 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_4096000", Name: "4.096 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_8192000", Name: "8.192 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_16384000", Name: "16.384 sec", Algo: module.PercentOfIncremental},
			{ID: "sync_proxy_rules_bucket_+Inf", Name: "+Inf", Algo: module.PercentOfIncremental},
		},
	},
	{
		ID:    "rest_client_requests_by_code",
		Title: "HTTP Requests By Status Code",
		Units: "requests/s",
		Fam:   "rest client",
		Ctx:   "k8s_kubeproxy.rest_client_requests_by_code",
		Type:  module.Stacked,
	},
	{
		ID:    "rest_client_requests_by_method",
		Title: "HTTP Requests By Status Method",
		Units: "requests/s",
		Fam:   "rest client",
		Ctx:   "k8s_kubeproxy.rest_client_requests_by_method",
		Type:  module.Stacked,
	},
	{
		ID:    "http_request_duration",
		Title: "HTTP Requests Duration",
		Units: "microseconds",
		Fam:   "http",
		Ctx:   "k8s_kubeproxy.http_request_duration",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "http_request_duration_05", Name: "0.5"},
			{ID: "http_request_duration_09", Name: "0.9"},
			{ID: "http_request_duration_099", Name: "0.99"},
		},
	},
}
