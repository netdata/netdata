// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux
// +build linux

package systemdunits

import (
	"fmt"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	prioServiceUnitState = module.Priority + iota
	prioSocketUnitState
	prioTargetUnitState
	prioPathUnitState
	prioDeviceUnitState
	prioMountUnitState
	prioAutomountUnitState
	prioSwapUnitState
	prioTimerUnitState
	prioScopeUnitState
	prioSliceUnitState
)

var prioMap = map[string]int{
	unitTypeService:   prioServiceUnitState,
	unitTypeSocket:    prioSocketUnitState,
	unitTypeTarget:    prioTargetUnitState,
	unitTypePath:      prioPathUnitState,
	unitTypeDevice:    prioDeviceUnitState,
	unitTypeMount:     prioMountUnitState,
	unitTypeAutomount: prioAutomountUnitState,
	unitTypeSwap:      prioSwapUnitState,
	unitTypeTimer:     prioTimerUnitState,
	unitTypeScope:     prioScopeUnitState,
	unitTypeSlice:     prioSliceUnitState,
}

func newTypedUnitStateChartTmpl(name, typ string) *module.Chart {
	chart := module.Chart{
		ID:       fmt.Sprintf("unit_%s_%s_state", name, typ),
		Title:    fmt.Sprintf("%s Unit State", cases.Title(language.English, cases.Compact).String(typ)),
		Units:    "state",
		Fam:      fmt.Sprintf("%s units", typ),
		Ctx:      fmt.Sprintf("systemd.%s_unit_state", typ),
		Priority: prioMap[typ],
		Labels: []module.Label{
			{Key: "unit_name", Value: name},
		},
		Dims: module.Dims{
			{Name: unitStateActive},
			{Name: unitStateInactive},
			{Name: unitStateActivating},
			{Name: unitStateDeactivating},
			{Name: unitStateFailed},
		},
	}
	for _, d := range chart.Dims {
		d.ID = fmt.Sprintf("unit_%s_%s_state_%s", name, typ, d.Name)
	}
	return &chart
}

func (s *SystemdUnits) addUnitToCharts(name, typ string) {
	chart := newTypedUnitStateChartTmpl(name, typ)

	if err := s.Charts().Add(chart); err != nil {
		s.Warning(err)
	}
}
