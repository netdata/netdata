// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// Job health belongs to the collecting Agent, independently of where the job's
// target charts live. Both runtimes settle this state through output admission.
type jobSelfMetrics struct {
	status, duration netdataapi.ChartOpts
	name             string
	labels           map[string]string
	published        bool
	durationUpdated  bool
	emission         jobSelfMetricsEmission
}

func newJobSelfMetrics(
	plugin, module, name, fullName string,
	updateEvery int,
	labels map[string]string,
) jobSelfMetrics {
	prefix := cleanPluginName(plugin) + "_" + fullName
	status := netdataapi.ChartOpts{
		TypeID:      "netdata",
		ID:          prefix + "_data_collection_status",
		Title:       "Data Collection Status",
		Units:       "status",
		Family:      plugin,
		Context:     "netdata.plugin_data_collection_status",
		ChartType:   "line",
		Priority:    144000,
		UpdateEvery: updateEvery,
		Plugin:      plugin,
		Module:      module,
	}
	duration := status
	duration.ID = prefix + "_data_collection_duration"
	duration.Title = "Data Collection Duration"
	duration.Units = "ms"
	duration.Context = "netdata.plugin_data_collection_duration"
	duration.Priority = 145000
	return jobSelfMetrics{
		status:   status,
		duration: duration,
		name:     name,
		labels:   labels,
	}
}

func (m *jobSelfMetrics) prepare(
	api *netdataapi.API,
	sinceLastRun int,
	elapsed int64,
	success, redefine bool,
) *jobSelfMetricsEmission {
	api.HOST("")
	if !m.published || redefine {
		m.createChart(api, m.status, "success", "failed")
		m.createChart(api, m.duration, "duration")
	}
	statusInterval := sinceLastRun
	if !m.published {
		statusInterval = 0
	}
	api.BEGIN("netdata", m.status.ID, statusInterval)
	if success {
		api.SET("success", 1)
		api.SET("failed", 0)
	} else {
		api.SET("success", 0)
		api.SET("failed", 1)
	}
	api.END()
	if success {
		durationInterval := sinceLastRun
		if !m.durationUpdated {
			durationInterval = 0
		}
		api.BEGIN("netdata", m.duration.ID, durationInterval)
		api.SET("duration", elapsed)
		api.END()
	}
	m.emission = jobSelfMetricsEmission{
		metrics:         m,
		durationUpdated: m.durationUpdated || success,
	}
	return &m.emission
}

func (m *jobSelfMetrics) createChart(api *netdataapi.API, chart netdataapi.ChartOpts, dimensions ...string) {
	api.CHART(chart)
	for key, value := range m.labels {
		api.CLABEL(key, lblValueReplacer.Replace(value), collectorapi.LabelSourceConf)
	}
	api.CLABEL("_collect_job", lblValueReplacer.Replace(m.name), collectorapi.LabelSourceAuto)
	api.CLABELCOMMIT()
	for _, id := range dimensions {
		api.DIMENSION(netdataapi.DimensionOpts{
			ID:         id,
			Algorithm:  "absolute",
			Multiplier: 1,
			Divisor:    1,
		})
	}
	_ = api.EMPTYLINE()
}

func (m *jobSelfMetrics) cleanup(api *netdataapi.API) {
	if !m.published {
		return
	}
	api.HOST("")
	for _, chart := range []netdataapi.ChartOpts{m.duration, m.status} {
		chart.Options = "obsolete"
		api.CHART(chart)
		_ = api.EMPTYLINE()
	}
}

func (m *jobSelfMetrics) clear() {
	m.published = false
	m.durationUpdated = false
	m.emission = jobSelfMetricsEmission{}
}

type jobSelfMetricsEmission struct {
	metrics         *jobSelfMetrics
	durationUpdated bool
	settled         bool
}

func (e *jobSelfMetricsEmission) Commit() error {
	if e != nil && !e.settled {
		e.metrics.published = true
		e.metrics.durationUpdated = e.durationUpdated
		e.settled = true
	}
	return nil
}

func (e *jobSelfMetricsEmission) Abort() error {
	if e != nil {
		e.settled = true
	}
	return nil
}
