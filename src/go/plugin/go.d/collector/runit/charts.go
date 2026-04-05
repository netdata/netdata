// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package runit

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioSummary = collectorapi.Priority + iota
	prioState
	prioStateDuration
)

var summaryCharts = collectorapi.Charts{
	{
		ID:       "services",
		Title:    "Services",
		Units:    "services",
		Fam:      "summary",
		Ctx:      "runit.services",
		Type:     collectorapi.Stacked,
		Priority: prioSummary,
		Dims: collectorapi.Dims{
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
	stateChart := &collectorapi.Chart{
		ID:       serviceStateChartID(service),
		Title:    "Service State",
		Units:    "state",
		Fam:      "state",
		Ctx:      "runit.service_state",
		Priority: prioState,
		Labels: []collectorapi.Label{
			{Key: "service_name", Value: service},
		},
	}
	chartID := stateChart.ID
	for _, s := range serviceStates {
		stateChart.Dims = append(stateChart.Dims, &collectorapi.Dim{
			ID:   serviceDimID(chartID, s),
			Name: s,
		})
	}
	stateChart.Dims = append(stateChart.Dims, &collectorapi.Dim{
		ID:      serviceDimID(chartID, stateEnabled),
		Name:    stateEnabled,
		DimOpts: collectorapi.DimOpts{Hidden: true},
	})

	durationChart := &collectorapi.Chart{
		ID:       serviceStateDurationChartID(service),
		Title:    "Service State Duration",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "runit.service_state_duration",
		Type:     collectorapi.Line,
		Priority: prioStateDuration,
		Labels: []collectorapi.Label{
			{Key: "service_name", Value: service},
		},
		Dims: collectorapi.Dims{
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
