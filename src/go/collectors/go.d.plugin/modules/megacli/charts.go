// SPDX-License-Identifier: GPL-3.0-or-later

package megacli

import (
	"fmt"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

const (
	prioAdapterHealthState = module.Priority + iota

	prioPhysDriveMediaErrorsRate
	prioPhysDrivePredictiveFailuresRate

	prioBBURelativeCharge
	prioBBURechargeCycles
	prioBBUTemperature
)

var adapterChartsTmpl = module.Charts{
	adapterHealthStateChartTmpl.Copy(),
}

var (
	adapterHealthStateChartTmpl = module.Chart{
		ID:       "adapter_%s_health_state",
		Title:    "Adapter health state",
		Units:    "state",
		Fam:      "adapter health",
		Ctx:      "megacli.adapter_health_state",
		Type:     module.Line,
		Priority: prioAdapterHealthState,
		Dims: module.Dims{
			{ID: "adapter_%s_health_state_optimal", Name: "optimal"},
			{ID: "adapter_%s_health_state_degraded", Name: "degraded"},
			{ID: "adapter_%s_health_state_partially_degraded", Name: "partially_degraded"},
			{ID: "adapter_%s_health_state_failed", Name: "failed"},
		},
	}
)

var physDriveChartsTmpl = module.Charts{
	physDriveMediaErrorsRateChartTmpl.Copy(),
	physDrivePredictiveFailuresRateChartTmpl.Copy(),
}

var (
	physDriveMediaErrorsRateChartTmpl = module.Chart{
		ID:       "phys_drive_%s_media_errors_rate",
		Title:    "Physical Drive media errors rate",
		Units:    "errors/s",
		Fam:      "phys drive errors",
		Ctx:      "megacli.phys_drive_media_errors",
		Type:     module.Line,
		Priority: prioPhysDriveMediaErrorsRate,
		Dims: module.Dims{
			{ID: "phys_drive_%s_media_error_count", Name: "media_errors"},
		},
	}
	physDrivePredictiveFailuresRateChartTmpl = module.Chart{
		ID:       "phys_drive_%s_predictive_failures_rate",
		Title:    "Physical Drive predictive failures rate",
		Units:    "failures/s",
		Fam:      "phys drive errors",
		Ctx:      "megacli.phys_drive_predictive_failures",
		Type:     module.Line,
		Priority: prioPhysDrivePredictiveFailuresRate,
		Dims: module.Dims{
			{ID: "phys_drive_%s_predictive_failure_count", Name: "predictive_failures"},
		},
	}
)

var bbuChartsTmpl = module.Charts{
	bbuRelativeChargeChartsTmpl.Copy(),
	bbuRechargeCyclesChartsTmpl.Copy(),
	bbuTemperatureChartsTmpl.Copy(),
}

var (
	bbuRelativeChargeChartsTmpl = module.Chart{
		ID:       "bbu_adapter_%s_relative_charge",
		Title:    "BBU relative charge",
		Units:    "percentage",
		Fam:      "bbu charge",
		Ctx:      "megacli.bbu_charge",
		Type:     module.Area,
		Priority: prioBBURelativeCharge,
		Dims: module.Dims{
			{ID: "bbu_adapter_%s_relative_state_of_charge", Name: "charge"},
		},
	}
	bbuRechargeCyclesChartsTmpl = module.Chart{
		ID:       "bbu_adapter_%s_recharge_cycles",
		Title:    "BBU recharge cycles",
		Units:    "cycles",
		Fam:      "bbu charge",
		Ctx:      "megacli.bbu_recharge_cycles",
		Type:     module.Line,
		Priority: prioBBURechargeCycles,
		Dims: module.Dims{
			{ID: "bbu_adapter_%s_cycle_count", Name: "recharge"},
		},
	}
	bbuTemperatureChartsTmpl = module.Chart{
		ID:       "bbu_adapter_%s_temperature",
		Title:    "BBU temperature",
		Units:    "Celsius",
		Fam:      "bbu temperature",
		Ctx:      "megacli.bbu_temperature",
		Type:     module.Line,
		Priority: prioBBUTemperature,
		Dims: module.Dims{
			{ID: "bbu_adapter_%s_temperature", Name: "temperature"},
		},
	}
)

func (m *MegaCli) addAdapterCharts(ad *megaAdapter) {
	charts := adapterChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, ad.number)
		chart.Labels = []module.Label{
			{Key: "adapter_number", Value: ad.number},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, ad.number)
		}
	}

	if err := m.Charts().Add(*charts...); err != nil {
		m.Warning(err)
	}
}

func (m *MegaCli) addPhysDriveCharts(pd *megaPhysDrive) {
	charts := physDriveChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, pd.wwn)
		chart.Labels = []module.Label{
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

	if err := m.Charts().Add(*charts...); err != nil {
		m.Warning(err)
	}
}

func (m *MegaCli) addBBUCharts(bbu *megaBBU) {
	charts := bbuChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, bbu.adapterNumber)
		chart.Labels = []module.Label{
			{Key: "adapter_number", Value: bbu.adapterNumber},
			{Key: "battery_type", Value: bbu.batteryType},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, bbu.adapterNumber)
		}
	}

	if err := m.Charts().Add(*charts...); err != nil {
		m.Warning(err)
	}
}
