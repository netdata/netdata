// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package systemdunits

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	prioUnitState = module.Priority + iota
	prioUnitFileState
)

func (c *Collector) addUnitCharts(name, typ string) {
	chart := module.Chart{
		ID:       "unit_%s_%s_state",
		Title:    "%s Unit State",
		Units:    "state",
		Fam:      "%s units",
		Ctx:      "systemd.%s_unit_state",
		Priority: prioUnitState,
		Labels: []module.Label{
			{Key: "unit_name", Value: name},
		},
		Dims: module.Dims{
			{ID: "unit_%s_%s_state_%s", Name: unitStateActive},
			{ID: "unit_%s_%s_state_%s", Name: unitStateInactive},
			{ID: "unit_%s_%s_state_%s", Name: unitStateActivating},
			{ID: "unit_%s_%s_state_%s", Name: unitStateDeactivating},
			{ID: "unit_%s_%s_state_%s", Name: unitStateFailed},
		},
	}

	chart.ID = fmt.Sprintf(chart.ID, name, typ)
	chart.Title = fmt.Sprintf(chart.Title, cases.Title(language.English, cases.Compact).String(typ))
	chart.Fam = fmt.Sprintf(chart.Fam, typ)
	chart.Ctx = fmt.Sprintf(chart.Ctx, typ)

	for _, d := range chart.Dims {
		d.ID = fmt.Sprintf(d.ID, name, typ, d.Name)
	}

	if err := c.Charts().Add(&chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeUnitCharts(name, typ string) {
	px := fmt.Sprintf("unit_%s_%s_", name, typ)
	c.removeCharts(px)
}

func (c *Collector) addUnitFileCharts(unitPath string) {
	_, unitName := filepath.Split(unitPath)
	unitType := strings.TrimPrefix(filepath.Ext(unitPath), ".")

	chart := module.Chart{
		ID:       "unit_file_%s_state",
		Title:    "Unit File State",
		Units:    "state",
		Fam:      "unit files",
		Ctx:      "systemd.unit_file_state",
		Type:     module.Line,
		Priority: prioUnitFileState,
		Labels: []module.Label{
			{Key: "unit_file_name", Value: unitName},
			{Key: "unit_file_type", Value: unitType},
		},
		Dims: module.Dims{
			{ID: "unit_file_%s_state_enabled", Name: "enabled"},
			{ID: "unit_file_%s_state_enabled-runtime", Name: "enabled-runtime"},
			{ID: "unit_file_%s_state_linked", Name: "linked"},
			{ID: "unit_file_%s_state_linked-runtime", Name: "linked-runtime"},
			{ID: "unit_file_%s_state_alias", Name: "alias"},
			{ID: "unit_file_%s_state_masked", Name: "masked"},
			{ID: "unit_file_%s_state_masked-runtime", Name: "masked-runtime"},
			{ID: "unit_file_%s_state_static", Name: "static"},
			{ID: "unit_file_%s_state_disabled", Name: "disabled"},
			{ID: "unit_file_%s_state_indirect", Name: "indirect"},
			{ID: "unit_file_%s_state_generated", Name: "generated"},
			{ID: "unit_file_%s_state_transient", Name: "transient"},
			{ID: "unit_file_%s_state_bad", Name: "bad"},
		},
	}

	chart.ID = fmt.Sprintf(chart.ID, strings.ReplaceAll(unitPath, ".", "_"))
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, unitPath)
	}

	if err := c.Charts().Add(&chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeUnitFileCharts(unitPath string) {
	px := fmt.Sprintf("unit_file_%s_", strings.ReplaceAll(unitPath, ".", "_"))
	c.removeCharts(px)
}

func (c *Collector) removeCharts(prefix string) {
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
