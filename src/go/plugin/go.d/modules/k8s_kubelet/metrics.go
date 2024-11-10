// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubelet

import (
	mtx "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

func newMetrics() *metrics {
	var mx metrics
	mx.RESTClient.Requests.ByStatusCode = make(map[string]mtx.Gauge)
	mx.RESTClient.Requests.ByMethod = make(map[string]mtx.Gauge)
	mx.Kubelet.Runtime.Operations = make(map[string]mtx.Gauge)
	mx.Kubelet.Runtime.OperationsErrors = make(map[string]mtx.Gauge)
	mx.Kubelet.Docker.Operations = make(map[string]mtx.Gauge)
	mx.Kubelet.Docker.OperationsErrors = make(map[string]mtx.Gauge)
	mx.Kubelet.PodLogFileSystemUsage = make(map[string]mtx.Gauge)

	return &mx
}

type metrics struct {
	Token         tokenMetrics         `stm:"token"`
	RESTClient    restClientMetrics    `stm:"rest_client"`
	APIServer     apiServerMetrics     `stm:"apiserver"`
	Kubelet       kubeletMetrics       `stm:"kubelet"`
	VolumeManager volumeManagerMetrics `stm:"volume_manager"`
}

type tokenMetrics struct {
	Count     mtx.Gauge `stm:"count"`
	FailCount mtx.Gauge `stm:"fail_count"`
}

type restClientMetrics struct {
	Requests struct {
		ByStatusCode map[string]mtx.Gauge `stm:""`
		ByMethod     map[string]mtx.Gauge `stm:""`
	} `stm:"requests"`
}

type apiServerMetrics struct {
	Audit struct {
		Requests struct {
			Rejected mtx.Gauge `stm:"rejected_total"`
		} `stm:"requests"`
	} `stm:"audit"`
	Storage struct {
		EnvelopeTransformation struct {
			CacheMisses mtx.Gauge `stm:"cache_misses_total"`
		} `stm:"envelope_transformation"`
		DataKeyGeneration struct {
			Failures  mtx.Gauge `stm:"failures_total"`
			Latencies struct {
				LE5     mtx.Gauge `stm:"5"`
				LE10    mtx.Gauge `stm:"10"`
				LE20    mtx.Gauge `stm:"20"`
				LE40    mtx.Gauge `stm:"40"`
				LE80    mtx.Gauge `stm:"80"`
				LE160   mtx.Gauge `stm:"160"`
				LE320   mtx.Gauge `stm:"320"`
				LE640   mtx.Gauge `stm:"640"`
				LE1280  mtx.Gauge `stm:"1280"`
				LE2560  mtx.Gauge `stm:"2560"`
				LE5120  mtx.Gauge `stm:"5120"`
				LE10240 mtx.Gauge `stm:"10240"`
				LE20480 mtx.Gauge `stm:"20480"`
				LE40960 mtx.Gauge `stm:"40960"`
				LEInf   mtx.Gauge `stm:"+Inf"`
			} `stm:"bucket"`
		} `stm:"data_key_generation"`
	} `stm:"storage"`
}

type kubeletMetrics struct {
	NodeConfigError       mtx.Gauge `stm:"node_config_error"`
	RunningContainerCount mtx.Gauge `stm:"running_container"`
	RunningPodCount       mtx.Gauge `stm:"running_pod"`
	PLEG                  struct {
		Relist struct {
			Interval struct {
				Quantile05  mtx.Gauge `stm:"05"`
				Quantile09  mtx.Gauge `stm:"09"`
				Quantile099 mtx.Gauge `stm:"099"`
			} `stm:"interval"`
			Latency struct {
				Quantile05  mtx.Gauge `stm:"05"`
				Quantile09  mtx.Gauge `stm:"09"`
				Quantile099 mtx.Gauge `stm:"099"`
			} `stm:"latency"`
		} `stm:"relist"`
	} `stm:"pleg"`
	Runtime struct {
		Operations       map[string]mtx.Gauge `stm:"operations"`
		OperationsErrors map[string]mtx.Gauge `stm:"operations_errors"`
	} `stm:"runtime"`
	Docker struct {
		Operations       map[string]mtx.Gauge `stm:"operations"`
		OperationsErrors map[string]mtx.Gauge `stm:"operations_errors"`
	} `stm:"docker"`
	PodLogFileSystemUsage map[string]mtx.Gauge `stm:"log_file_system_usage"`
}

type volumeManagerMetrics struct {
	Plugins map[string]*volumeManagerPlugin `stm:"plugin"`
}

type volumeManagerPlugin struct {
	State struct {
		Actual  mtx.Gauge `stm:"actual"`
		Desired mtx.Gauge `stm:"desired"`
	} `stm:"state"`
}
