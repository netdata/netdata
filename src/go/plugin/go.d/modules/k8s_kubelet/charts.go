// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubelet

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

type (
	// Charts is an alias for module.Charts
	Charts = module.Charts
	// Chart is an alias for module.Chart
	Chart = module.Chart
	// Dims is an alias for module.Dims
	Dims = module.Dims
	// Dim is an alias for module.Dim
	Dim = module.Dim
)

var charts = Charts{
	{
		ID:    "apiserver_audit_requests_rejected_total",
		Title: "API Server Audit Requests",
		Units: "requests/s",
		Fam:   "api server",
		Ctx:   "k8s_kubelet.apiserver_audit_requests_rejected",
		Dims: Dims{
			{ID: "apiserver_audit_requests_rejected_total", Name: "rejected", Algo: module.Incremental},
		},
	},
	{
		ID:    "apiserver_storage_data_key_generation_failures_total",
		Title: "API Server Failed Data Encryption Key(DEK) Generation Operations",
		Units: "events/s",
		Fam:   "api server",
		Ctx:   "k8s_kubelet.apiserver_storage_data_key_generation_failures",
		Dims: Dims{
			{ID: "apiserver_storage_data_key_generation_failures_total", Name: "failures", Algo: module.Incremental},
		},
	},
	{
		ID:    "apiserver_storage_data_key_generation_latencies",
		Title: "API Server Latencies Of Data Encryption Key(DEK) Generation Operations",
		Units: "observes/s",
		Fam:   "api server",
		Ctx:   "k8s_kubelet.apiserver_storage_data_key_generation_latencies",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "apiserver_storage_data_key_generation_bucket_5", Name: "5 µs", Algo: module.Incremental},
			{ID: "apiserver_storage_data_key_generation_bucket_10", Name: "10 µs", Algo: module.Incremental},
			{ID: "apiserver_storage_data_key_generation_bucket_20", Name: "20 µs", Algo: module.Incremental},
			{ID: "apiserver_storage_data_key_generation_bucket_40", Name: "40 µs", Algo: module.Incremental},
			{ID: "apiserver_storage_data_key_generation_bucket_80", Name: "80 µs", Algo: module.Incremental},
			{ID: "apiserver_storage_data_key_generation_bucket_160", Name: "160 µs", Algo: module.Incremental},
			{ID: "apiserver_storage_data_key_generation_bucket_320", Name: "320 µs", Algo: module.Incremental},
			{ID: "apiserver_storage_data_key_generation_bucket_640", Name: "640 µs", Algo: module.Incremental},
			{ID: "apiserver_storage_data_key_generation_bucket_1280", Name: "1280 µs", Algo: module.Incremental},
			{ID: "apiserver_storage_data_key_generation_bucket_2560", Name: "2560 µs", Algo: module.Incremental},
			{ID: "apiserver_storage_data_key_generation_bucket_5120", Name: "5120 µs", Algo: module.Incremental},
			{ID: "apiserver_storage_data_key_generation_bucket_10240", Name: "10240 µs", Algo: module.Incremental},
			{ID: "apiserver_storage_data_key_generation_bucket_20480", Name: "20480 µs", Algo: module.Incremental},
			{ID: "apiserver_storage_data_key_generation_bucket_40960", Name: "40960 µs", Algo: module.Incremental},
			{ID: "apiserver_storage_data_key_generation_bucket_+Inf", Name: "+Inf", Algo: module.Incremental},
		},
	},
	{
		ID:    "apiserver_storage_data_key_generation_latencies_percentage",
		Title: "API Server Latencies Of Data Encryption Key(DEK) Generation Operations Percentage",
		Units: "%",
		Fam:   "api server",
		Ctx:   "k8s_kubelet.apiserver_storage_data_key_generation_latencies_percent",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "apiserver_storage_data_key_generation_bucket_5", Name: "5 µs", Algo: module.PercentOfIncremental},
			{ID: "apiserver_storage_data_key_generation_bucket_10", Name: "10 µs", Algo: module.PercentOfIncremental},
			{ID: "apiserver_storage_data_key_generation_bucket_20", Name: "20 µs", Algo: module.PercentOfIncremental},
			{ID: "apiserver_storage_data_key_generation_bucket_40", Name: "40 µs", Algo: module.PercentOfIncremental},
			{ID: "apiserver_storage_data_key_generation_bucket_80", Name: "80 µs", Algo: module.PercentOfIncremental},
			{ID: "apiserver_storage_data_key_generation_bucket_160", Name: "160 µs", Algo: module.PercentOfIncremental},
			{ID: "apiserver_storage_data_key_generation_bucket_320", Name: "320 µs", Algo: module.PercentOfIncremental},
			{ID: "apiserver_storage_data_key_generation_bucket_640", Name: "640 µs", Algo: module.PercentOfIncremental},
			{ID: "apiserver_storage_data_key_generation_bucket_1280", Name: "1280 µs", Algo: module.PercentOfIncremental},
			{ID: "apiserver_storage_data_key_generation_bucket_2560", Name: "2560 µs", Algo: module.PercentOfIncremental},
			{ID: "apiserver_storage_data_key_generation_bucket_5120", Name: "5120 µs", Algo: module.PercentOfIncremental},
			{ID: "apiserver_storage_data_key_generation_bucket_10240", Name: "10240 µs", Algo: module.PercentOfIncremental},
			{ID: "apiserver_storage_data_key_generation_bucket_20480", Name: "20480 µs", Algo: module.PercentOfIncremental},
			{ID: "apiserver_storage_data_key_generation_bucket_40960", Name: "40960 µs", Algo: module.PercentOfIncremental},
			{ID: "apiserver_storage_data_key_generation_bucket_+Inf", Name: "+Inf", Algo: module.PercentOfIncremental},
		},
	},
	{
		ID:    "apiserver_storage_envelope_transformation_cache_misses_total",
		Title: "API Server Storage Envelope Transformation Cache Misses",
		Units: "events/s",
		Fam:   "api server",
		Ctx:   "k8s_kubelet.apiserver_storage_envelope_transformation_cache_misses",
		Dims: Dims{
			{ID: "apiserver_storage_envelope_transformation_cache_misses_total", Name: "cache misses", Algo: module.Incremental},
		},
	},
	{
		ID:    "kubelet_containers_running",
		Title: "Number Of Containers Currently Running",
		Units: "running containers",
		Fam:   "containers",
		Ctx:   "k8s_kubelet.kubelet_containers_running",
		Dims: Dims{
			{ID: "kubelet_running_container", Name: "total"},
		},
	},
	{
		ID:    "kubelet_pods_running",
		Title: "Number Of Pods Currently Running",
		Units: "running pods",
		Fam:   "pods",
		Ctx:   "k8s_kubelet.kubelet_pods_running",
		Dims: Dims{
			{ID: "kubelet_running_pod", Name: "total"},
		},
	},
	{
		ID:    "kubelet_pods_log_filesystem_used_bytes",
		Title: "Bytes Used By The Pod Logs On The Filesystem",
		Units: "B",
		Fam:   "pods",
		Ctx:   "k8s_kubelet.kubelet_pods_log_filesystem_used_bytes",
		Type:  module.Stacked,
	},
	{
		ID:    "kubelet_runtime_operations",
		Title: "Runtime Operations By Type",
		Units: "operations/s",
		Fam:   "operations",
		Ctx:   "k8s_kubelet.kubelet_runtime_operations",
		Type:  module.Stacked,
	},
	{
		ID:    "kubelet_runtime_operations_errors",
		Title: "Runtime Operations Errors By Type",
		Units: "errors/s",
		Fam:   "operations",
		Ctx:   "k8s_kubelet.kubelet_runtime_operations_errors",
		Type:  module.Stacked,
	},
	{
		ID:    "kubelet_docker_operations",
		Title: "Docker Operations By Type",
		Units: "operations/s",
		Fam:   "operations",
		Ctx:   "k8s_kubelet.kubelet_docker_operations",
		Type:  module.Stacked,
	},
	{
		ID:    "kubelet_docker_operations_errors",
		Title: "Docker Operations Errors By Type",
		Units: "errors/s",
		Fam:   "operations",
		Ctx:   "k8s_kubelet.kubelet_docker_operations_errors",
		Type:  module.Stacked,
	},
	{
		ID:    "kubelet_node_config_error",
		Title: "Node Configuration-Related Error",
		Units: "bool",
		Fam:   "config error",
		Ctx:   "k8s_kubelet.kubelet_node_config_error",
		Dims: Dims{
			{ID: "kubelet_node_config_error", Name: "experiencing_error"},
		},
	},
	{
		ID:    "kubelet_pleg_relist_interval_microseconds",
		Title: "PLEG Relisting Interval Summary",
		Units: "microseconds",
		Fam:   "pleg relisting",
		Ctx:   "k8s_kubelet.kubelet_pleg_relist_interval_microseconds",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "kubelet_pleg_relist_interval_05", Name: "0.5"},
			{ID: "kubelet_pleg_relist_interval_09", Name: "0.9"},
			{ID: "kubelet_pleg_relist_interval_099", Name: "0.99"},
		},
	},
	{
		ID:    "kubelet_pleg_relist_latency_microseconds",
		Title: "PLEG Relisting Latency Summary",
		Units: "microseconds",
		Fam:   "pleg relisting",
		Ctx:   "k8s_kubelet.kubelet_pleg_relist_latency_microseconds",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "kubelet_pleg_relist_latency_05", Name: "0.5"},
			{ID: "kubelet_pleg_relist_latency_09", Name: "0.9"},
			{ID: "kubelet_pleg_relist_latency_099", Name: "0.99"},
		},
	},
	{
		ID:    "kubelet_token_requests",
		Title: "Token() Requests To The Alternate Token Source",
		Units: "token requests/s",
		Fam:   "token",
		Ctx:   "k8s_kubelet.kubelet_token_requests",
		Dims: Dims{
			{ID: "token_count", Name: "total", Algo: module.Incremental},
			{ID: "token_fail_count", Name: "failed", Algo: module.Incremental},
		},
	},
	{
		ID:    "rest_client_requests_by_code",
		Title: "HTTP Requests By Status Code",
		Units: "requests/s",
		Fam:   "rest client",
		Ctx:   "k8s_kubelet.rest_client_requests_by_code",
		Type:  module.Stacked,
	},
	{
		ID:    "rest_client_requests_by_method",
		Title: "HTTP Requests By Status Method",
		Units: "requests/s",
		Fam:   "rest client",
		Ctx:   "k8s_kubelet.rest_client_requests_by_method",
		Type:  module.Stacked,
	},
}

func newVolumeManagerChart(name string) *Chart {
	return &Chart{
		ID:    "volume_manager_total_volumes_" + name,
		Title: "Volume Manager State Of The World, Plugin " + name,
		Units: "state",
		Fam:   "volume manager",
		Ctx:   "k8s_kubelet.volume_manager_total_volumes",
		Dims: Dims{
			{ID: "volume_manager_plugin_" + name + "_state_actual", Name: "actual"},
			{ID: "volume_manager_plugin_" + name + "_state_desired", Name: "desired"},
		},
	}
}
