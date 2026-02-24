// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioStratum = collectorapi.Priority + iota
	prioCurrentCorrection
	prioRootDelay
	prioRootDispersion
	prioLastOffset
	prioRmsOffset
	prioFrequency
	prioResidualFrequency
	prioSkew
	prioUpdateInterval
	prioRefMeasurementTime
	prioLeapStatus
	prioActivity
	prioNTPPackets
	prioCommandPackets
)

var charts = collectorapi.Charts{
	stratumChart.Copy(),

	currentCorrectionChart.Copy(),

	rootDelayChart.Copy(),
	rootDispersionChart.Copy(),

	lastOffsetChart.Copy(),
	rmsOffsetChart.Copy(),

	frequencyChart.Copy(),
	residualFrequencyChart.Copy(),

	skewChart.Copy(),

	updateIntervalChart.Copy(),
	refMeasurementTimeChart.Copy(),

	leapStatusChart.Copy(),

	activityChart.Copy(),
}

// Tracking charts
var (
	stratumChart = collectorapi.Chart{
		ID:       "stratum",
		Title:    "Distance to the reference clock",
		Units:    "level",
		Fam:      "stratum",
		Ctx:      "chrony.stratum",
		Priority: prioStratum,
		Dims: collectorapi.Dims{
			{ID: "stratum", Name: "stratum"},
		},
	}

	currentCorrectionChart = collectorapi.Chart{
		ID:       "current_correction",
		Title:    "Current correction",
		Units:    "seconds",
		Fam:      "correction",
		Ctx:      "chrony.current_correction",
		Priority: prioCurrentCorrection,
		Dims: collectorapi.Dims{
			{ID: "current_correction", Div: scaleFactor},
		},
	}

	rootDelayChart = collectorapi.Chart{
		ID:       "root_delay",
		Title:    "Network path delay to stratum-1",
		Units:    "seconds",
		Fam:      "root",
		Ctx:      "chrony.root_delay",
		Priority: prioRootDelay,
		Dims: collectorapi.Dims{
			{ID: "root_delay", Div: scaleFactor},
		},
	}
	rootDispersionChart = collectorapi.Chart{
		ID:       "root_dispersion",
		Title:    "Dispersion accumulated back to stratum-1",
		Units:    "seconds",
		Fam:      "root",
		Ctx:      "chrony.root_dispersion",
		Priority: prioRootDispersion,
		Dims: collectorapi.Dims{
			{ID: "root_dispersion", Div: scaleFactor},
		},
	}

	lastOffsetChart = collectorapi.Chart{
		ID:       "last_offset",
		Title:    "Offset on the last clock update",
		Units:    "seconds",
		Fam:      "offset",
		Ctx:      "chrony.last_offset",
		Priority: prioLastOffset,
		Dims: collectorapi.Dims{
			{ID: "last_offset", Name: "offset", Div: scaleFactor},
		},
	}
	rmsOffsetChart = collectorapi.Chart{
		ID:       "rms_offset",
		Title:    "Long-term average of the offset value",
		Units:    "seconds",
		Fam:      "offset",
		Ctx:      "chrony.rms_offset",
		Priority: prioRmsOffset,
		Dims: collectorapi.Dims{
			{ID: "rms_offset", Name: "offset", Div: scaleFactor},
		},
	}

	frequencyChart = collectorapi.Chart{
		ID:       "frequency",
		Title:    "Frequency",
		Units:    "ppm",
		Fam:      "frequency",
		Ctx:      "chrony.frequency",
		Priority: prioFrequency,
		Dims: collectorapi.Dims{
			{ID: "frequency", Div: scaleFactor},
		},
	}
	residualFrequencyChart = collectorapi.Chart{
		ID:       "residual_frequency",
		Title:    "Residual frequency",
		Units:    "ppm",
		Fam:      "frequency",
		Ctx:      "chrony.residual_frequency",
		Priority: prioResidualFrequency,
		Dims: collectorapi.Dims{
			{ID: "residual_frequency", Div: scaleFactor},
		},
	}

	skewChart = collectorapi.Chart{
		ID:       "skew",
		Title:    "Skew",
		Units:    "ppm",
		Fam:      "frequency",
		Ctx:      "chrony.skew",
		Priority: prioSkew,
		Dims: collectorapi.Dims{
			{ID: "skew", Div: scaleFactor},
		},
	}

	updateIntervalChart = collectorapi.Chart{
		ID:       "update_interval",
		Title:    "Interval between the last two clock updates",
		Units:    "seconds",
		Fam:      "updates",
		Ctx:      "chrony.update_interval",
		Priority: prioUpdateInterval,
		Dims: collectorapi.Dims{
			{ID: "update_interval", Div: scaleFactor},
		},
	}
	refMeasurementTimeChart = collectorapi.Chart{
		ID:       "ref_measurement_time",
		Title:    "Time since the last measurement",
		Units:    "seconds",
		Fam:      "updates",
		Ctx:      "chrony.ref_measurement_time",
		Priority: prioRefMeasurementTime,
		Dims: collectorapi.Dims{
			{ID: "ref_measurement_time"},
		},
	}

	leapStatusChart = collectorapi.Chart{
		ID:       "leap_status",
		Title:    "Leap status",
		Units:    "status",
		Fam:      "leap status",
		Ctx:      "chrony.leap_status",
		Priority: prioLeapStatus,
		Dims: collectorapi.Dims{
			{ID: "leap_status_normal", Name: "normal"},
			{ID: "leap_status_insert_second", Name: "insert_second"},
			{ID: "leap_status_delete_second", Name: "delete_second"},
			{ID: "leap_status_unsynchronised", Name: "unsynchronised"},
		},
	}
)

// Activity charts
var (
	activityChart = collectorapi.Chart{
		ID:       "activity",
		Title:    "Peers activity",
		Units:    "sources",
		Fam:      "activity",
		Ctx:      "chrony.activity",
		Type:     collectorapi.Stacked,
		Priority: prioActivity,
		Dims: collectorapi.Dims{
			{ID: "online_sources", Name: "online"},
			{ID: "offline_sources", Name: "offline"},
			{ID: "burst_online_sources", Name: "burst_online"},
			{ID: "burst_offline_sources", Name: "burst_offline"},
			{ID: "unresolved_sources", Name: "unresolved"},
		},
	}
)

var serverStatsCharts = collectorapi.Charts{
	ntpPacketsChart.Copy(),
	commandPacketsChart.Copy(),
}

var (
	ntpPacketsChart = collectorapi.Chart{
		ID:       "ntp_packets",
		Title:    "NTP packets",
		Units:    "packets/s",
		Fam:      "client requests",
		Ctx:      "chrony.ntp_packets",
		Type:     collectorapi.Line,
		Priority: prioNTPPackets,
		Dims: collectorapi.Dims{
			{ID: "ntp_packets_received", Name: "received", Algo: collectorapi.Incremental},
			{ID: "ntp_packets_dropped", Name: "dropped", Algo: collectorapi.Incremental},
		},
	}
	commandPacketsChart = collectorapi.Chart{
		ID:       "command_packets",
		Title:    "Command packets",
		Units:    "packets/s",
		Fam:      "client requests",
		Ctx:      "chrony.command_packets",
		Type:     collectorapi.Line,
		Priority: prioCommandPackets,
		Dims: collectorapi.Dims{
			{ID: "command_packets_received", Name: "received", Algo: collectorapi.Incremental},
			{ID: "command_packets_dropped", Name: "dropped", Algo: collectorapi.Incremental},
		},
	}
)

func (c *Collector) addServerStatsCharts() {
	if err := c.Charts().Add(*serverStatsCharts.Copy()...); err != nil {
		c.Warning(err)
	}
}
