// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubeproxy

import (
	mtx "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

func newMetrics() *metrics {
	var mx metrics
	mx.RESTClient.Requests.ByStatusCode = make(map[string]mtx.Gauge)
	mx.RESTClient.Requests.ByMethod = make(map[string]mtx.Gauge)

	return &mx
}

type metrics struct {
	SyncProxyRules struct {
		Count   mtx.Gauge `stm:"count"`
		Latency struct {
			LE1000     mtx.Gauge `stm:"1000"`
			LE2000     mtx.Gauge `stm:"2000"`
			LE4000     mtx.Gauge `stm:"4000"`
			LE8000     mtx.Gauge `stm:"8000"`
			LE16000    mtx.Gauge `stm:"16000"`
			LE32000    mtx.Gauge `stm:"32000"`
			LE64000    mtx.Gauge `stm:"64000"`
			LE128000   mtx.Gauge `stm:"128000"`
			LE256000   mtx.Gauge `stm:"256000"`
			LE512000   mtx.Gauge `stm:"512000"`
			LE1024000  mtx.Gauge `stm:"1024000"`
			LE2048000  mtx.Gauge `stm:"2048000"`
			LE4096000  mtx.Gauge `stm:"4096000"`
			LE8192000  mtx.Gauge `stm:"8192000"`
			LE16384000 mtx.Gauge `stm:"16384000"`
			Inf        mtx.Gauge `stm:"+Inf"`
		} `stm:"bucket"`
	} `stm:"sync_proxy_rules"`
	RESTClient struct {
		Requests struct {
			ByStatusCode map[string]mtx.Gauge `stm:""`
			ByMethod     map[string]mtx.Gauge `stm:""`
		} `stm:"requests"`
	} `stm:"rest_client"`
	HTTP struct {
		Request struct {
			Duration struct {
				Quantile05  mtx.Gauge `stm:"05"`
				Quantile09  mtx.Gauge `stm:"09"`
				Quantile099 mtx.Gauge `stm:"099"`
			} `stm:"duration"`
		} `stm:"request"`
	} `stm:"http"`
}
