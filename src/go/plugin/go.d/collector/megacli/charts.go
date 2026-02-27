// SPDX-License-Identifier: GPL-3.0-or-later

package megacli

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioAdapterHealthState = collectorapi.Priority + iota

	prioPhysDriveMediaErrorsRate
	prioPhysDrivePredictiveFailuresRate

	prioBBURelativeCharge
	prioBBURechargeCycles
	prioBBUCapDegradationPerc
	prioBBUTemperature
)

var adapterChartsTmpl = collectorapi.Charts{
	adapterHealthStateChartTmpl.Copy(),
}

var (
	adapterHealthStateChartTmpl = collectorapi.Chart{
		ID:       "adapter_%s_health_state",
		Title:    "Adapter health state",
		Units:    "state",
		Fam:      "adapter health",
		Ctx:      "megacli.adapter_health_state",
		Type:     collectorapi.Line,
		Priority: prioAdapterHealthState,
		Dims: collectorapi.Dims{
			{ID: "adapter_%s_health_state_optimal", Name: "optimal"},
			{ID: "adapter_%s_health_state_degraded", Name: "degraded"},
			{ID: "adapter_%s_health_state_partially_degraded", Name: "partially_degraded"},
			{ID: "adapter_%s_health_state_failed", Name: "failed"},
		},
	}
)

var physDriveChartsTmpl = collectorapi.Charts{
	physDriveMediaErrorsRateChartTmpl.Copy(),
	physDrivePredictiveFailuresRateChartTmpl.Copy(),
}

var (
	physDriveMediaErrorsRateChartTmpl = collectorapi.Chart{
		ID:       "phys_drive_%s_media_errors_rate",
		Title:    "Physical Drive media errors rate",
		Units:    "errors/s",
		Fam:      "phys drive errors",
		Ctx:      "megacli.phys_drive_media_errors",
		Type:     collectorapi.Line,
		Priority: prioPhysDriveMediaErrorsRate,
		Dims: collectorapi.Dims{
			{ID: "phys_drive_%s_media_error_count", Name: "media_errors"},
		},
	}
	physDrivePredictiveFailuresRateChartTmpl = collectorapi.Chart{
		ID:       "phys_drive_%s_predictive_failures_rate",
		Title:    "Physical Drive predictive failures rate",
		Units:    "failures/s",
		Fam:      "phys drive errors",
		Ctx:      "megacli.phys_drive_predictive_failures",
		Type:     collectorapi.Line,
		Priority: prioPhysDrivePredictiveFailuresRate,
		Dims: collectorapi.Dims{
			{ID: "phys_drive_%s_predictive_failure_count", Name: "predictive_failures"},
		},
	}
)

var bbuChartsTmpl = collectorapi.Charts{
	bbuRelativeChargeChartsTmpl.Copy(),
	bbuRechargeCyclesChartsTmpl.Copy(),
	bbuCapacityDegradationChartsTmpl.Copy(),
	bbuTemperatureChartsTmpl.Copy(),
}

var (
	bbuRelativeChargeChartsTmpl = collectorapi.Chart{
		ID:       "bbu_adapter_%s_relative_charge",
		Title:    "BBU relative charge",
		Units:    "percentage",
		Fam:      "bbu charge",
		Ctx:      "megacli.bbu_charge",
		Type:     collectorapi.Area,
		Priority: prioBBURelativeCharge,
		Dims: collectorapi.Dims{
			{ID: "bbu_adapter_%s_relative_state_of_charge", Name: "charge"},
		},
	}
	bbuRechargeCyclesChartsTmpl = collectorapi.Chart{
		ID:       "bbu_adapter_%s_recharge_cycles",
		Title:    "BBU recharge cycles",
		Units:    "cycles",
		Fam:      "bbu charge",
		Ctx:      "megacli.bbu_recharge_cycles",
		Type:     collectorapi.Line,
		Priority: prioBBURechargeCycles,
		Dims: collectorapi.Dims{
			{ID: "bbu_adapter_%s_cycle_count", Name: "recharge"},
		},
	}
	bbuCapacityDegradationChartsTmpl = collectorapi.Chart{
		ID:       "bbu_adapter_%s_capacity_degradation",
		Title:    "BBU capacity degradation",
		Units:    "percent",
		Fam:      "bbu charge",
		Ctx:      "megacli.bbu_capacity_degradation",
		Type:     collectorapi.Line,
		Priority: prioBBUCapDegradationPerc,
		Dims: collectorapi.Dims{
			{ID: "bbu_adapter_%s_capacity_degradation_perc", Name: "cap_degradation"},
		},
	}
	bbuTemperatureChartsTmpl = collectorapi.Chart{
		ID:       "bbu_adapter_%s_temperature",
		Title:    "BBU temperature",
		Units:    "Celsius",
		Fam:      "bbu temperature",
		Ctx:      "megacli.bbu_temperature",
		Type:     collectorapi.Line,
		Priority: prioBBUTemperature,
		Dims: collectorapi.Dims{
			{ID: "bbu_adapter_%s_temperature", Name: "temperature"},
		},
	}
)

func (c *Collector) addAdapterCharts(ad *megaAdapter) {
	charts := adapterChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, ad.number)
		chart.Labels = []collectorapi.Label{
			{Key: "adapter_number", Value: ad.number},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, ad.number)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addPhysDriveCharts(pd *megaPhysDrive) {
	charts := physDriveChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, pd.wwn)
		chart.Labels = []collectorapi.Label{
			{Key: "adapter_number", Value: pd.adapterNumber},
			{Key: "wwn", Value: pd.wwn},
			{Key: "slot_number", Value: pd.slotNumber},
			{Key: "drive_position", Value: pd.drivePosition},
			{Key: "drive_type", Value: pd.pdType},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, pd.wwn)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addBBUCharts(bbu *megaBBU) {
	charts := bbuChartsTmpl.Copy()

	if _, ok := calcCapDegradationPerc(bbu); !ok {
		_ = charts.Remove(bbuCapacityDegradationChartsTmpl.ID)
	}

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, bbu.adapterNumber)
		chart.Labels = []collectorapi.Label{
			{Key: "adapter_number", Value: bbu.adapterNumber},
			{Key: "battery_type", Value: bbu.batteryType},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, bbu.adapterNumber)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}
