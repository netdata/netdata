// SPDX-License-Identifier: GPL-3.0-or-later

package uwsgi

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioTransmittedData = collectorapi.Priority + iota
	prioRequests
	prioHarakiris
	prioExceptions
	prioRespawns

	prioWorkerTransmittedData
	prioWorkerRequests
	prioWorkerDeltaRequests
	prioWorkerAvgRequestTime
	prioWorkerHarakiris
	prioWorkerExceptions
	prioWorkerStatus
	prioWorkerRequestHandlingStatus
	prioWorkerRespawns
	prioWorkerMemoryRss
	prioWorkerMemoryVsz
)

var charts = collectorapi.Charts{
	transmittedDataChart.Copy(),
	requestsChart.Copy(),
	harakirisChart.Copy(),
	exceptionsChart.Copy(),
	respawnsChart.Copy(),
}

var (
	transmittedDataChart = collectorapi.Chart{
		ID:       "transmitted_data",
		Title:    "UWSGI Transmitted Data",
		Units:    "bytes/s",
		Fam:      "workers",
		Ctx:      "uwsgi.transmitted_data",
		Priority: prioTransmittedData,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "workers_tx", Name: "tx", Algo: collectorapi.Incremental},
		},
	}
	requestsChart = collectorapi.Chart{
		ID:       "requests",
		Title:    "UWSGI Requests",
		Units:    "requests/s",
		Fam:      "workers",
		Ctx:      "uwsgi.requests",
		Priority: prioRequests,
		Dims: collectorapi.Dims{
			{ID: "workers_requests", Name: "requests", Algo: collectorapi.Incremental},
		},
	}
	harakirisChart = collectorapi.Chart{
		ID:       "harakiris",
		Title:    "UWSGI Dropped Requests",
		Units:    "harakiris/s",
		Fam:      "workers",
		Ctx:      "uwsgi.harakiris",
		Priority: prioHarakiris,
		Dims: collectorapi.Dims{
			{ID: "workers_harakiris", Name: "harakiris", Algo: collectorapi.Incremental},
		},
	}
	exceptionsChart = collectorapi.Chart{
		ID:       "exceptions",
		Title:    "UWSGI Raised Exceptions",
		Units:    "exceptions/s",
		Fam:      "workers",
		Ctx:      "uwsgi.exceptions",
		Priority: prioExceptions,
		Dims: collectorapi.Dims{
			{ID: "workers_exceptions", Name: "exceptions", Algo: collectorapi.Incremental},
		},
	}
	respawnsChart = collectorapi.Chart{
		ID:       "respawns",
		Title:    "UWSGI Respawns",
		Units:    "respawns/s",
		Fam:      "workers",
		Ctx:      "uwsgi.respawns",
		Priority: prioRespawns,
		Dims: collectorapi.Dims{
			{ID: "workers_respawns", Name: "respawns", Algo: collectorapi.Incremental},
		},
	}
)

var workerChartsTmpl = collectorapi.Charts{
	workerTransmittedDataChartTmpl.Copy(),
	workerRequestsChartTmpl.Copy(),
	workerDeltaRequestsChartTmpl.Copy(),
	workerAvgRequestTimeChartTmpl.Copy(),
	workerHarakirisChartTmpl.Copy(),
	workerExceptionsChartTmpl.Copy(),
	workerStatusChartTmpl.Copy(),
	workerRequestHandlingStatusChartTmpl.Copy(),
	workerRespawnsChartTmpl.Copy(),
	workerMemoryRssChartTmpl.Copy(),
	workerMemoryVszChartTmpl.Copy(),
}

var (
	workerTransmittedDataChartTmpl = collectorapi.Chart{
		ID:       "worker_%s_transmitted_data",
		Title:    "UWSGI Worker Transmitted Data",
		Units:    "bytes/s",
		Fam:      "wrk transmitted data",
		Ctx:      "uwsgi.worker_transmitted_data",
		Priority: prioWorkerTransmittedData,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "worker_%s_tx", Name: "tx", Algo: collectorapi.Incremental},
		},
	}
	workerRequestsChartTmpl = collectorapi.Chart{
		ID:       "worker_%s_requests",
		Title:    "UWSGI Worker Requests",
		Units:    "requests/s",
		Fam:      "wrk requests",
		Ctx:      "uwsgi.worker_requests",
		Priority: prioWorkerRequests,
		Dims: collectorapi.Dims{
			{ID: "worker_%s_requests", Name: "requests", Algo: collectorapi.Incremental},
		},
	}
	workerDeltaRequestsChartTmpl = collectorapi.Chart{
		ID:       "worker_%s_delta_requests",
		Title:    "UWSGI Worker Delta Requests",
		Units:    "requests/s",
		Fam:      "wrk requests",
		Ctx:      "uwsgi.worker_delta_requests",
		Priority: prioWorkerDeltaRequests,
		Dims: collectorapi.Dims{
			{ID: "worker_%s_delta_requests", Name: "delta_requests", Algo: collectorapi.Incremental},
		},
	}
	workerAvgRequestTimeChartTmpl = collectorapi.Chart{
		ID:       "worker_%s_average_request_time",
		Title:    "UWSGI Worker Average Request Time",
		Units:    "milliseconds",
		Fam:      "wrk request time",
		Ctx:      "uwsgi.worker_average_request_time",
		Priority: prioWorkerAvgRequestTime,
		Dims: collectorapi.Dims{
			{ID: "worker_%s_average_request_time", Name: "avg"},
		},
	}
	workerHarakirisChartTmpl = collectorapi.Chart{
		ID:       "worker_%s_harakiris",
		Title:    "UWSGI Worker Dropped Requests",
		Units:    "harakiris/s",
		Fam:      "wrk harakiris",
		Ctx:      "uwsgi.worker_harakiris",
		Priority: prioWorkerHarakiris,
		Dims: collectorapi.Dims{
			{ID: "worker_%s_harakiris", Name: "harakiris", Algo: collectorapi.Incremental},
		},
	}
	workerExceptionsChartTmpl = collectorapi.Chart{
		ID:       "worker_%s_exceptions",
		Title:    "UWSGI Worker Raised Exceptions",
		Units:    "exceptions/s",
		Fam:      "wrk exceptions",
		Ctx:      "uwsgi.worker_exceptions",
		Priority: prioWorkerExceptions,
		Dims: collectorapi.Dims{
			{ID: "worker_%s_exceptions", Name: "exceptions", Algo: collectorapi.Incremental},
		},
	}
	workerStatusChartTmpl = collectorapi.Chart{
		ID:       "worker_%s_status",
		Title:    "UWSGI Worker Status",
		Units:    "status",
		Fam:      "wrk status",
		Ctx:      "uwsgi.status",
		Priority: prioWorkerStatus,
		Dims: collectorapi.Dims{
			{ID: "worker_%s_status_idle", Name: "idle"},
			{ID: "worker_%s_status_busy", Name: "busy"},
			{ID: "worker_%s_status_cheap", Name: "cheap"},
			{ID: "worker_%s_status_pause", Name: "pause"},
			{ID: "worker_%s_status_sig", Name: "sig"},
		},
	}
	workerRequestHandlingStatusChartTmpl = collectorapi.Chart{
		ID:       "worker_%s_request_handling_status",
		Title:    "UWSGI Worker Request Handling Status",
		Units:    "status",
		Fam:      "wrk status",
		Ctx:      "uwsgi.request_handling_status",
		Priority: prioWorkerRequestHandlingStatus,
		Dims: collectorapi.Dims{
			{ID: "worker_%s_request_handling_status_accepting", Name: "accepting"},
			{ID: "worker_%s_request_handling_status_not_accepting", Name: "not_accepting"},
		},
	}
	workerRespawnsChartTmpl = collectorapi.Chart{
		ID:       "worker_%s_respawns",
		Title:    "UWSGI Worker Respawns",
		Units:    "respawns/s",
		Fam:      "wrk respawns",
		Ctx:      "uwsgi.worker_respawns",
		Priority: prioWorkerRespawns,
		Dims: collectorapi.Dims{
			{ID: "worker_%s_respawns", Name: "respawns", Algo: collectorapi.Incremental},
		},
	}
	workerMemoryRssChartTmpl = collectorapi.Chart{
		ID:       "worker_%s_memory_rss",
		Title:    "UWSGI Worker Memory RSS (Resident Set Size)",
		Units:    "bytes",
		Fam:      "wrk memory",
		Ctx:      "uwsgi.worker_memory_rss",
		Priority: prioWorkerMemoryRss,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "worker_%s_memory_rss", Name: "rss"},
		},
	}
	workerMemoryVszChartTmpl = collectorapi.Chart{
		ID:       "worker_%s_memory_vsz",
		Title:    "UWSGI Worker Memory VSZ (Virtual Memory Size)",
		Units:    "bytes",
		Fam:      "wrk memory",
		Ctx:      "uwsgi.worker_memory_vsz",
		Priority: prioWorkerMemoryVsz,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "worker_%s_memory_vsz", Name: "vsz"},
		},
	}
)

func (c *Collector) addWorkerCharts(workerID int) {
	charts := workerChartsTmpl.Copy()

	id := strconv.Itoa(workerID)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, id)
		chart.Labels = []collectorapi.Label{
			{Key: "worker_id", Value: id},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, id)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeWorkerCharts(workerID int) {
	px := fmt.Sprintf("worker_%d_", workerID)

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
