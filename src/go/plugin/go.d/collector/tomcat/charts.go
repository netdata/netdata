// SPDX-License-Identifier: GPL-3.0-or-later

package tomcat

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioConnectorRequestsCount = collectorapi.Priority + iota
	prioConnectorRequestsBandwidth
	prioConnectorRequestsProcessingTime
	prioConnectorRequestsErrors

	prioConnectorRequestThreads

	prioJvmMemoryUsage

	prioJvmMemoryPoolMemoryUsage
)

var (
	defaultCharts = collectorapi.Charts{
		jvmMemoryUsageChart.Copy(),
	}

	jvmMemoryUsageChart = collectorapi.Chart{
		ID:       "jvm_memory_usage",
		Title:    "JVM Memory Usage",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "tomcat.jvm_memory_usage",
		Type:     collectorapi.Stacked,
		Priority: prioJvmMemoryUsage,
		Dims: collectorapi.Dims{
			{ID: "jvm_memory_free", Name: "free"},
			{ID: "jvm_memory_used", Name: "used"},
		},
	}
)

var (
	connectorChartsTmpl = collectorapi.Charts{
		connectorRequestsCountChartTmpl.Copy(),
		connectorRequestsBandwidthChartTmpl.Copy(),
		connectorRequestsProcessingTimeChartTmpl.Copy(),
		connectorRequestsErrorsChartTmpl.Copy(),
		connectorRequestThreadsChartTmpl.Copy(),
	}

	connectorRequestsCountChartTmpl = collectorapi.Chart{
		ID:       "connector_%_requests",
		Title:    "Connector Requests",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "tomcat.connector_requests",
		Type:     collectorapi.Line,
		Priority: prioConnectorRequestsCount,
		Dims: collectorapi.Dims{
			{ID: "connector_%s_request_info_request_count", Name: "requests", Algo: collectorapi.Incremental},
		},
	}
	connectorRequestsBandwidthChartTmpl = collectorapi.Chart{
		ID:       "connector_%s_requests_bandwidth",
		Title:    "Connector Requests Bandwidth",
		Units:    "bytes/s",
		Fam:      "requests",
		Ctx:      "tomcat.connector_bandwidth",
		Type:     collectorapi.Area,
		Priority: prioConnectorRequestsBandwidth,
		Dims: collectorapi.Dims{
			{ID: "connector_%s_request_info_bytes_received", Name: "received", Algo: collectorapi.Incremental},
			{ID: "connector_%s_request_info_bytes_sent", Name: "sent", Mul: -1, Algo: collectorapi.Incremental},
		},
	}
	connectorRequestsProcessingTimeChartTmpl = collectorapi.Chart{
		ID:       "connector_%_requests_processing_time",
		Title:    "Connector Requests Processing Time",
		Units:    "milliseconds",
		Fam:      "requests",
		Ctx:      "tomcat.connector_requests_processing_time",
		Type:     collectorapi.Line,
		Priority: prioConnectorRequestsProcessingTime,
		Dims: collectorapi.Dims{
			{ID: "connector_%s_request_info_processing_time", Name: "processing_time", Algo: collectorapi.Incremental},
		},
	}
	connectorRequestsErrorsChartTmpl = collectorapi.Chart{
		ID:       "connector_%_errors",
		Title:    "Connector Errors",
		Units:    "errors/s",
		Fam:      "requests",
		Ctx:      "tomcat.connector_errors",
		Type:     collectorapi.Line,
		Priority: prioConnectorRequestsErrors,
		Dims: collectorapi.Dims{
			{ID: "connector_%s_request_info_error_count", Name: "errors", Algo: collectorapi.Incremental},
		},
	}

	connectorRequestThreadsChartTmpl = collectorapi.Chart{
		ID:       "connector_%s_request_threads",
		Title:    "Connector RequestConfig Threads",
		Units:    "threads",
		Fam:      "threads",
		Ctx:      "tomcat.connector_request_threads",
		Type:     collectorapi.Stacked,
		Priority: prioConnectorRequestThreads,
		Dims: collectorapi.Dims{
			{ID: "connector_%s_thread_info_idle", Name: "idle"},
			{ID: "connector_%s_thread_info_busy", Name: "busy"},
		},
	}
)

var (
	jvmMemoryPoolChartsTmpl = collectorapi.Charts{
		jvmMemoryPoolMemoryUsageChartTmpl.Copy(),
	}

	jvmMemoryPoolMemoryUsageChartTmpl = collectorapi.Chart{
		ID:       "jvm_mem_pool_%s_memory_usage",
		Title:    "JVM Mem Pool Memory Usage",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "tomcat.jvm_mem_pool_memory_usage",
		Type:     collectorapi.Area,
		Priority: prioJvmMemoryPoolMemoryUsage,
		Dims: collectorapi.Dims{
			{ID: "jvm_memorypool_%s_commited", Name: "commited"},
			{ID: "jvm_memorypool_%s_used", Name: "used"},
			{ID: "jvm_memorypool_%s_max", Name: "max"},
		},
	}
)

func (c *Collector) addConnectorCharts(name string) {
	charts := connectorChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName(name))
		chart.Labels = []collectorapi.Label{
			{Key: "connector_name", Value: strings.Trim(name, "\"")},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName(name))
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addMemPoolCharts(name, typ string) {
	name = strings.ReplaceAll(name, "'", "")

	charts := jvmMemoryPoolChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName(name))
		chart.Labels = []collectorapi.Label{
			{Key: "mempool_name", Value: name},
			{Key: "mempool_type", Value: typ},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName(name))
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeConnectorCharts(name string) {
	px := fmt.Sprintf("connector_%s_", cleanName(name))
	c.removeCharts(px)
}

func (c *Collector) removeMemoryPoolCharts(name string) {
	px := fmt.Sprintf("jvm_mem_pool_%s_", cleanName(name))
	c.removeCharts(px)
}

func (c *Collector) removeCharts(prefix string) {
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
