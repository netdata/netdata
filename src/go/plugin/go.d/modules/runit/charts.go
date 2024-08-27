// SPDX-License-Identifier: GPL-3.0-or-later

package runit

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

type (
	ChartID   string
	DimIDElem string
)

const (
	prioState = module.Priority + iota
	prioStateDuration
)

const (
	dimStateDown       DimIDElem = "down"
	dimStateDownWantUp DimIDElem = "starting"
	dimStateRun        DimIDElem = "up"
	dimStateFinish     DimIDElem = "finishing"
	dimStatePaused     DimIDElem = "paused"
	dimStateEnabled    DimIDElem = "enabled"
)

func stateChartID(service string) ChartID {
	return ChartID(fmt.Sprintf("service_%s_state", service))
}

func (s *Runit) newStateChart(service string) *module.Chart {
	chart := &module.Chart{
		ID:       string(stateChartID(service)),
		Title:    "Service State",
		Units:    "state",
		Fam:      "state",
		Ctx:      "runit.service_state",
		Type:     module.Stacked,
		Priority: prioState,
		Labels: []module.Label{
			{Key: "service", Value: service},
		},
	}
	addDim(chart, dimStateDown)
	addDim(chart, dimStateDownWantUp)
	addDim(chart, dimStateRun)
	addDim(chart, dimStateFinish)
	addDim(chart, dimStatePaused)
	addDim(chart, dimStateEnabled).Hidden = true
	return chart
}

func stateDurationChartID(service string) ChartID {
	return ChartID(fmt.Sprintf("service_%s_state_duration", service))
}

func (s *Runit) newStateDurationChart(service string) *module.Chart {
	chart := &module.Chart{
		ID:       string(stateDurationChartID(service)),
		Title:    "Service State Duration",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "runit.service_state_duration",
		Type:     module.Line,
		Priority: prioStateDuration,
		Labels: []module.Label{
			{Key: "service", Value: service},
		},
	}
	addDim(chart, dimStateDown)
	addDim(chart, dimStateRun)
	return chart
}

func addDim(chart *module.Chart, dimIDElem DimIDElem) *module.Dim {
	dim := &module.Dim{
		ID:   dimID(ChartID(chart.ID), dimIDElem),
		Name: string(dimIDElem),
	}
	chart.Dims = append(chart.Dims, dim)
	return dim
}

func dimID(chartID ChartID, dim DimIDElem) string {
	return fmt.Sprintf("%s:%s", chartID, dim)
}

func (s *Runit) addStateCharts(service string) {
	s.addChart(s.newStateChart(service))
	s.addChart(s.newStateDurationChart(service))
}

func (s *Runit) delStateCharts(service string) {
	s.delChart(stateChartID(service))
	s.delChart(stateDurationChartID(service))
}

func (s *Runit) addChart(chart *module.Chart) {
	err := s.Charts().Add(chart)
	if err != nil {
		s.Warningf("addChart(%q): %v", chart.ID, err)
	}
}

func (s *Runit) delChart(delID ChartID) {
	found := 0
	for _, chart := range *s.Charts() {
		alreadyRemoved := chart.Obsolete // XXX For now only MarkRemove sets Obsolete.
		if ChartID(chart.ID) == delID && !alreadyRemoved {
			chart.MarkRemove()
			chart.MarkNotCreated()
			found++
		}
	}
	if found != 1 {
		s.Warningf("delChart(%q): removed %d charts instead of 1", delID, found)
	}
}
