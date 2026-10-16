// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubelet

import (
	"math"
	"strings"

	"context"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	mtx "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"
)

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	raw, err := c.prom.ScrapeSeries(ctx)

	if err != nil {
		return nil, err
	}

	mx := newMetrics()

	c.collectToken(raw, mx)
	c.collectRESTClientHTTPRequests(raw, mx)
	c.collectAPIServer(raw, mx)
	c.collectKubelet(raw, mx)
	c.collectVolumeManager(raw, mx)
	c.collectVolumeStats(raw, mx)

	return stm.ToMap(mx), nil
}

func (c *Collector) collectLogsUsagePerPod(raw prometheus.Series, mx *metrics) {
	chart := c.charts.Get("kubelet_pods_log_filesystem_used_bytes")
	seen := make(map[string]bool)

	for _, metric := range raw.FindByName("kubelet_container_log_filesystem_used_bytes") {
		pod := metric.Labels.Get("pod")
		namespace := metric.Labels.Get("namespace")

		if pod == "" || namespace == "" {
			continue
		}

		key := namespace + "_" + pod
		dimID := "kubelet_log_file_system_usage_" + key

		if !chart.HasDim(dimID) {
			_ = chart.AddDim(&Dim{ID: dimID, Name: pod})
			chart.MarkNotCreated()
		}

		seen[dimID] = true
		v := mx.Kubelet.PodLogFileSystemUsage[key]
		v.Add(metric.Value)
		mx.Kubelet.PodLogFileSystemUsage[key] = v
	}

	for _, dim := range chart.Dims {
		if seen[dim.ID] {
			continue
		}
		_ = chart.MarkDimRemove(dim.ID, false)
		chart.MarkNotCreated()
	}
}

func (c *Collector) collectVolumeManager(raw prometheus.Series, mx *metrics) {
	vmPlugins := make(map[string]*volumeManagerPlugin)

	for _, metric := range raw.FindByName("volume_manager_total_volumes") {
		pluginName := metric.Labels.Get("plugin_name")
		state := metric.Labels.Get("state")

		if !c.collectedVMPlugins[pluginName] {
			_ = c.charts.Add(newVolumeManagerChart(pluginName))
			c.collectedVMPlugins[pluginName] = true
		}
		if _, ok := vmPlugins[pluginName]; !ok {
			vmPlugins[pluginName] = &volumeManagerPlugin{}
		}

		switch state {
		case "actual_state_of_world":
			vmPlugins[pluginName].State.Actual.Set(metric.Value)
		case "desired_state_of_world":
			vmPlugins[pluginName].State.Desired.Set(metric.Value)
		}
	}

	mx.VolumeManager.Plugins = vmPlugins
}

// pvcStats holds the raw kubelet_volume_stats_* values of a PVC.
type pvcStats struct {
	namespace, name string

	capacity, used, available      float64
	inodes, inodesUsed, inodesFree float64
}

func (c *Collector) collectVolumeStats(raw prometheus.Series, mx *metrics) {
	/*
		# HELP kubelet_volume_stats_available_bytes [ALPHA] Number of available bytes in the volume
		# TYPE kubelet_volume_stats_available_bytes gauge
		kubelet_volume_stats_available_bytes{namespace="app",persistentvolumeclaim="data-db-0"} 450000
	*/
	stats := make(map[string]*pvcStats)

	series := raw.FindByNames(
		"kubelet_volume_stats_capacity_bytes",
		"kubelet_volume_stats_used_bytes",
		"kubelet_volume_stats_available_bytes",
		"kubelet_volume_stats_inodes",
		"kubelet_volume_stats_inodes_used",
		"kubelet_volume_stats_inodes_free",
	)

	for _, metric := range series {
		namespace := metric.Labels.Get("namespace")
		pvc := metric.Labels.Get("persistentvolumeclaim")

		if namespace == "" || pvc == "" {
			continue
		}

		key := namespace + "_" + pvc
		st, ok := stats[key]
		if !ok {
			st = &pvcStats{namespace: namespace, name: pvc}
			stats[key] = st
		}

		switch metric.Name() {
		case "kubelet_volume_stats_capacity_bytes":
			st.capacity = metric.Value
		case "kubelet_volume_stats_used_bytes":
			st.used = metric.Value
		case "kubelet_volume_stats_available_bytes":
			st.available = metric.Value
		case "kubelet_volume_stats_inodes":
			st.inodes = metric.Value
		case "kubelet_volume_stats_inodes_used":
			st.inodesUsed = metric.Value
		case "kubelet_volume_stats_inodes_free":
			st.inodesFree = metric.Value
		}
	}

	for key, st := range stats {
		if _, ok := c.collectedPVCs[key]; !ok {
			charts := newPVCCharts(st.namespace, st.name)
			if err := c.charts.Add(*charts...); err != nil {
				c.Warningf("failed to add charts for PVC '%s/%s': %v", st.namespace, st.name, err)
				continue
			}
			c.collectedPVCs[key] = charts
		}

		// the kubelet reads the values separately, so they can be slightly inconsistent
		pvc := &pvcMetrics{}
		pvc.Space.Available.Set(st.available)
		pvc.Space.Used.Set(st.used)
		pvc.Space.Reserved.Set(max(0, st.capacity-st.used-st.available))
		if total := st.used + st.available; total > 0 {
			pvc.Space.Utilization = &pvcUtilization{}
			pvc.Space.Utilization.Percent.Set(st.used / total * 100)
		}
		pvc.Inodes.Available.Set(st.inodesFree)
		pvc.Inodes.Used.Set(st.inodesUsed)
		pvc.Inodes.Reserved.Set(max(0, st.inodes-st.inodesUsed-st.inodesFree))

		mx.PVC[key] = pvc
	}

	for key, charts := range c.collectedPVCs {
		if _, ok := stats[key]; !ok {
			removePVCCharts(charts)
			delete(c.collectedPVCs, key)
		}
	}
}

func (c *Collector) collectKubelet(raw prometheus.Series, mx *metrics) {
	value := raw.FindByName("kubelet_node_config_error").Max()
	mx.Kubelet.NodeConfigError.Set(value)

	/*
		# HELP kubelet_running_containers [ALPHA] Number of containers currently running
		# TYPE kubelet_running_containers gauge
		kubelet_running_containers{container_state="created"} 1
		kubelet_running_containers{container_state="exited"} 13
		kubelet_running_containers{container_state="running"} 42
		kubelet_running_containers{container_state="unknown"} 1
	*/

	ms := raw.FindByName("kubelet_running_container_count")
	value = ms.Max()
	if ms.Len() == 0 {
		for _, m := range raw.FindByName("kubelet_running_containers") {
			if m.Labels.Get("container_state") == "running" {
				value = m.Value
				break
			}
		}
	}
	mx.Kubelet.RunningContainerCount.Set(value)

	/*
		# HELP kubelet_running_pods [ALPHA] Number of pods currently running
		# TYPE kubelet_running_pods gauge
		kubelet_running_pods 37
	*/
	value = raw.FindByNames("kubelet_running_pod_count", "kubelet_running_pods").Max()
	mx.Kubelet.RunningPodCount.Set(value)

	c.collectRuntimeOperations(raw, mx)
	c.collectRuntimeOperationsErrors(raw, mx)
	c.collectDockerOperations(raw, mx)
	c.collectDockerOperationsErrors(raw, mx)
	c.collectPLEGRelisting(raw, mx)
	c.collectLogsUsagePerPod(raw, mx)
}

func (c *Collector) collectAPIServer(raw prometheus.Series, mx *metrics) {
	value := raw.FindByName("apiserver_audit_requests_rejected_total").Max()
	mx.APIServer.Audit.Requests.Rejected.Set(value)

	value = raw.FindByName("apiserver_storage_data_key_generation_failures_total").Max()
	mx.APIServer.Storage.DataKeyGeneration.Failures.Set(value)

	value = raw.FindByName("apiserver_storage_envelope_transformation_cache_misses_total").Max()
	mx.APIServer.Storage.EnvelopeTransformation.CacheMisses.Set(value)

	c.collectStorageDataKeyGenerationLatencies(raw, mx)
}

func (c *Collector) collectToken(raw prometheus.Series, mx *metrics) {
	value := raw.FindByName("get_token_count").Max()
	mx.Token.Count.Set(value)

	value = raw.FindByName("get_token_fail_count").Max()
	mx.Token.FailCount.Set(value)
}

func (c *Collector) collectPLEGRelisting(raw prometheus.Series, mx *metrics) {
	// Summary
	for _, metric := range raw.FindByName("kubelet_pleg_relist_interval_microseconds") {
		if math.IsNaN(metric.Value) {
			continue
		}
		quantile := metric.Labels.Get("quantile")
		switch quantile {
		case "0.5":
			mx.Kubelet.PLEG.Relist.Interval.Quantile05.Set(metric.Value)
		case "0.9":
			mx.Kubelet.PLEG.Relist.Interval.Quantile09.Set(metric.Value)
		case "0.99":
			mx.Kubelet.PLEG.Relist.Interval.Quantile099.Set(metric.Value)
		}
	}
	for _, metric := range raw.FindByName("kubelet_pleg_relist_latency_microseconds") {
		if math.IsNaN(metric.Value) {
			continue
		}
		quantile := metric.Labels.Get("quantile")
		switch quantile {
		case "0.5":
			mx.Kubelet.PLEG.Relist.Latency.Quantile05.Set(metric.Value)
		case "0.9":
			mx.Kubelet.PLEG.Relist.Latency.Quantile09.Set(metric.Value)
		case "0.99":
			mx.Kubelet.PLEG.Relist.Latency.Quantile099.Set(metric.Value)
		}
	}
}

func (c *Collector) collectStorageDataKeyGenerationLatencies(raw prometheus.Series, mx *metrics) {
	latencies := &mx.APIServer.Storage.DataKeyGeneration.Latencies
	metricName := "apiserver_storage_data_key_generation_latencies_microseconds_bucket"

	for _, metric := range raw.FindByName(metricName) {
		value := metric.Value
		bucket, _ := strings.CutSuffix(metric.Labels.Get("le"), ".0")
		switch bucket {
		case "5":
			latencies.LE5.Set(value)
		case "10":
			latencies.LE10.Set(value)
		case "20":
			latencies.LE20.Set(value)
		case "40":
			latencies.LE40.Set(value)
		case "80":
			latencies.LE80.Set(value)
		case "160":
			latencies.LE160.Set(value)
		case "320":
			latencies.LE320.Set(value)
		case "640":
			latencies.LE640.Set(value)
		case "1280":
			latencies.LE1280.Set(value)
		case "2560":
			latencies.LE2560.Set(value)
		case "5120":
			latencies.LE5120.Set(value)
		case "10240":
			latencies.LE10240.Set(value)
		case "20480":
			latencies.LE20480.Set(value)
		case "40960":
			latencies.LE40960.Set(value)
		case "+Inf":
			latencies.LEInf.Set(value)
		}
	}

	latencies.LEInf.Sub(latencies.LE40960.Value())
	latencies.LE40960.Sub(latencies.LE20480.Value())
	latencies.LE20480.Sub(latencies.LE10240.Value())
	latencies.LE10240.Sub(latencies.LE5120.Value())
	latencies.LE5120.Sub(latencies.LE2560.Value())
	latencies.LE2560.Sub(latencies.LE1280.Value())
	latencies.LE1280.Sub(latencies.LE640.Value())
	latencies.LE640.Sub(latencies.LE320.Value())
	latencies.LE320.Sub(latencies.LE160.Value())
	latencies.LE160.Sub(latencies.LE80.Value())
	latencies.LE80.Sub(latencies.LE40.Value())
	latencies.LE40.Sub(latencies.LE20.Value())
	latencies.LE20.Sub(latencies.LE10.Value())
	latencies.LE10.Sub(latencies.LE5.Value())
}

func (c *Collector) collectRESTClientHTTPRequests(raw prometheus.Series, mx *metrics) {
	metricName := "rest_client_requests_total"
	chart := c.charts.Get("rest_client_requests_by_code")

	for _, metric := range raw.FindByName(metricName) {
		code := metric.Labels.Get("code")
		if code == "" {
			continue
		}
		dimID := "rest_client_requests_" + code
		if !chart.HasDim(dimID) {
			_ = chart.AddDim(&Dim{ID: dimID, Name: code, Algo: collectorapi.Incremental})
			chart.MarkNotCreated()
		}
		// a code has one series per method and host: sum them
		v := mx.RESTClient.Requests.ByStatusCode[code]
		v.Add(metric.Value)
		mx.RESTClient.Requests.ByStatusCode[code] = v
	}

	chart = c.charts.Get("rest_client_requests_by_method")

	for _, metric := range raw.FindByName(metricName) {
		method := metric.Labels.Get("method")
		if method == "" {
			continue
		}
		dimID := "rest_client_requests_" + method
		if !chart.HasDim(dimID) {
			_ = chart.AddDim(&Dim{ID: dimID, Name: method, Algo: collectorapi.Incremental})
			chart.MarkNotCreated()
		}
		v := mx.RESTClient.Requests.ByMethod[method]
		v.Add(metric.Value)
		mx.RESTClient.Requests.ByMethod[method] = v
	}
}

func (c *Collector) collectRuntimeOperations(raw prometheus.Series, mx *metrics) {
	chart := c.charts.Get("kubelet_runtime_operations")

	// kubelet_runtime_operations_total
	for _, metric := range raw.FindByNames("kubelet_runtime_operations", "kubelet_runtime_operations_total") {
		opType := metric.Labels.Get("operation_type")
		if opType == "" {
			continue
		}
		dimID := "kubelet_runtime_operations_" + opType
		if !chart.HasDim(dimID) {
			_ = chart.AddDim(&Dim{ID: dimID, Name: opType, Algo: collectorapi.Incremental})
			chart.MarkNotCreated()
		}
		mx.Kubelet.Runtime.Operations[opType] = mtx.Gauge(metric.Value)
	}
}

func (c *Collector) collectRuntimeOperationsErrors(raw prometheus.Series, mx *metrics) {
	chart := c.charts.Get("kubelet_runtime_operations_errors")

	// kubelet_runtime_operations_errors_total
	for _, metric := range raw.FindByNames("kubelet_runtime_operations_errors", "kubelet_runtime_operations_errors_total") {
		opType := metric.Labels.Get("operation_type")
		if opType == "" {
			continue
		}
		dimID := "kubelet_runtime_operations_errors_" + opType
		if !chart.HasDim(dimID) {
			_ = chart.AddDim(&Dim{ID: dimID, Name: opType, Algo: collectorapi.Incremental})
			chart.MarkNotCreated()
		}
		mx.Kubelet.Runtime.OperationsErrors[opType] = mtx.Gauge(metric.Value)
	}
}

func (c *Collector) collectDockerOperations(raw prometheus.Series, mx *metrics) {
	chart := c.charts.Get("kubelet_docker_operations")

	// kubelet_docker_operations_total
	for _, metric := range raw.FindByNames("kubelet_docker_operations", "kubelet_docker_operations_total") {
		opType := metric.Labels.Get("operation_type")
		if opType == "" {
			continue
		}
		dimID := "kubelet_docker_operations_" + opType
		if !chart.HasDim(dimID) {
			_ = chart.AddDim(&Dim{ID: dimID, Name: opType, Algo: collectorapi.Incremental})
			chart.MarkNotCreated()
		}
		mx.Kubelet.Docker.Operations[opType] = mtx.Gauge(metric.Value)
	}
}

func (c *Collector) collectDockerOperationsErrors(raw prometheus.Series, mx *metrics) {
	chart := c.charts.Get("kubelet_docker_operations_errors")

	// kubelet_docker_operations_errors_total
	for _, metric := range raw.FindByNames("kubelet_docker_operations_errors", "kubelet_docker_operations_errors_total") {
		opType := metric.Labels.Get("operation_type")
		if opType == "" {
			continue
		}
		dimID := "kubelet_docker_operations_errors_" + opType
		if !chart.HasDim(dimID) {
			_ = chart.AddDim(&Dim{ID: dimID, Name: opType, Algo: collectorapi.Incremental})
			chart.MarkNotCreated()
		}
		mx.Kubelet.Docker.OperationsErrors[opType] = mtx.Gauge(metric.Value)
	}
}
