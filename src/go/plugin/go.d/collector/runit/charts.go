// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package runit

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioSummary = module.Priority + iota
	prioState
	prioStateDuration
)

var summaryCharts = module.Charts{
	{
		ID:       "services",
		Title:    "Services",
		Units:    "services",
		Fam:      "summary",
		Ctx:      "runit.services",
		Type:     module.Stacked,
		Priority: prioSummary,
		Dims: module.Dims{
			{ID: "running_services", Name: "running"},
			{ID: "not_running_services", Name: "not_running"},
		},
	},
}

const (
	stateRunning  = "running"
	stateDown     = "down"
	stateFailed   = "failed"
	stateStarting = "starting"
	stateStopping = "stopping"
	statePaused   = "paused"
	stateEnabled  = "enabled" // hidden, for alerts
)

var serviceStates = []string{
	stateRunning,
	stateDown,
	stateFailed,
	stateStarting,
	stateStopping,
	statePaused,
}

func serviceStateChartID(service string) string {
	return fmt.Sprintf("service_%s_state", service)
}

func serviceStateDurationChartID(service string) string {
	return fmt.Sprintf("service_%s_state_duration", service)
}

func serviceDimID(chartID, state string) string {
	return fmt.Sprintf("%s_%s", chartID, state)
}

func (c *Collector) addServiceCharts(service string) {
	stateChart := &module.Chart{
		ID:       serviceStateChartID(service),
		Title:    "Service State",
		Units:    "state",
		Fam:      "state",
		Ctx:      "runit.service_state",
		Priority: prioState,
		Labels: []module.Label{
			{Key: "service_name", Value: service},
		},
	}
	chartID := stateChart.ID
	for _, s := range serviceStates {
		stateChart.Dims = append(stateChart.Dims, &module.Dim{
			ID:   serviceDimID(chartID, s),
			Name: s,
		})
	}
	stateChart.Dims = append(stateChart.Dims, &module.Dim{
		ID:      serviceDimID(chartID, stateEnabled),
		Name:    stateEnabled,
		DimOpts: module.DimOpts{Hidden: true},
	})

	durationChart := &module.Chart{
		ID:       serviceStateDurationChartID(service),
		Title:    "Service State Duration",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "runit.service_state_duration",
		Type:     module.Line,
		Priority: prioStateDuration,
		Labels: []module.Label{
			{Key: "service_name", Value: service},
		},
		Dims: module.Dims{
			{ID: serviceDimID(serviceStateDurationChartID(service), stateDown), Name: stateDown},
			{ID: serviceDimID(serviceStateDurationChartID(service), stateRunning), Name: stateRunning},
		},
	}

	if err := c.Charts().Add(stateChart, durationChart); err != nil {
		c.Warningf("add service charts for %q: %v", service, err)
	}
}

func (c *Collector) removeServiceCharts(service string) {
	stateID := serviceStateChartID(service)
	durationID := serviceStateDurationChartID(service)
	for _, chart := range *c.Charts() {
		if chart.ID == stateID || chart.ID == durationID {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
