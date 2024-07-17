// SPDX-License-Identifier: GPL-3.0-or-later

package icecast

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioListeners = module.Priority + iota
)

var chartsTmpl = module.Charts{
	ListenersChart.Copy(),
}

var (
	ListenersChart = module.Chart{
		ID:       "icecast_%s_listeners",
		Title:    "Number of Icecast Listeners on active sources",
		Units:    "listeners",
		Fam:      "listeners",
		Ctx:      "icecast.listeners",
		Type:     module.Line,
		Priority: prioListeners,
		Dims: module.Dims{
			{ID: "%s_listeners", Name: "listeners"},
		},
	}
)

func (a *Icecast) addSourceChart(stats *Source) {
	chart := ListenersChart.Copy()

	chart.ID = fmt.Sprintf(chart.ID, stats.ServerName)
	chart.Labels = []module.Label{
		{Key: "source", Value: stats.ServerName},
	}
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, stats.ServerName)
	}

	if err := a.Charts().Add(chart); err != nil {
		a.Warning(err)
	}

}

func (a *Icecast) removeSourceChart(source *Source) {
	px := fmt.Sprintf("icecast_%s_listeners", source.ServerName)
	for _, chart := range *a.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
