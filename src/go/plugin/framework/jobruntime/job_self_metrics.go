// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"time"

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
	// Run times of each chart's last delivered sample, zero before the first.
	// The Agent places a sample by the interval it carries, so the interval
	// spans every run since that sample: the duration chart skips failed runs,
	// and any run whose output was rejected delivered nothing.
	statusAt, durationAt time.Time
	emission             jobSelfMetricsEmission
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

// prepare writes the run's samples; run is the time the run started.
func (m *jobSelfMetrics) prepare(
	api *netdataapi.API,
	run time.Time,
	elapsed int64,
	success, redefine bool,
) *jobSelfMetricsEmission {
	api.HOST("")
	if !m.published || redefine {
		m.createChart(api, m.status, "success", "failed")
		m.createChart(api, m.duration, "duration")
	}
	api.BEGIN("netdata", m.status.ID, calcSinceLastRun(run, m.statusAt))
	if success {
		api.SET("success", 1)
		api.SET("failed", 0)
	} else {
		api.SET("success", 0)
		api.SET("failed", 1)
	}
	api.END()
	if success {
		api.BEGIN("netdata", m.duration.ID, calcSinceLastRun(run, m.durationAt))
		api.SET("duration", elapsed)
		api.END()
	}
	m.emission = jobSelfMetricsEmission{
		metrics: m,
		run:     run,
		success: success,
	}
	return &m.emission
}

func (m *jobSelfMetrics) createChart(api *netdataapi.API, chart netdataapi.ChartOpts, dimensions ...string) {
	api.CHART(chart)
	for key, value := range m.labels {
		api.CLABEL(netdataapi.SanitizeWireValue(key), netdataapi.SanitizeWireValue(value), collectorapi.LabelSourceConf)
	}
	api.CLABEL("_collect_job", netdataapi.SanitizeWireValue(m.name), collectorapi.LabelSourceAuto)
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
	m.statusAt = time.Time{}
	m.durationAt = time.Time{}
	m.emission = jobSelfMetricsEmission{}
}

type jobSelfMetricsEmission struct {
	metrics *jobSelfMetrics
	run     time.Time
	success bool
	settled bool
}

func (e *jobSelfMetricsEmission) Commit() error {
	if e != nil && !e.settled {
		e.metrics.published = true
		e.metrics.statusAt = e.run
		if e.success {
			e.metrics.durationAt = e.run
		}
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
