// SPDX-License-Identifier: GPL-3.0-or-later

package icecast

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioSourceListeners = module.Priority + iota
)

var sourceChartsTmpl = module.Charts{
	sourceListenersChartTmpl.Copy(),
}

var (
	sourceListenersChartTmpl = module.Chart{
		ID:       "icecast_%s_listeners",
		Title:    "Icecast Listeners",
		Units:    "listeners",
		Fam:      "listeners",
		Ctx:      "icecast.listeners",
		Type:     module.Line,
		Priority: prioSourceListeners,
		Dims: module.Dims{
			{ID: "source_%s_listeners", Name: "listeners"},
		},
	}
)

func (c *Collector) addSourceCharts(name string) {
	chart := sourceListenersChartTmpl.Copy()

	chart.ID = fmt.Sprintf(chart.ID, cleanSource(name))
	chart.Labels = []module.Label{
		{Key: "source", Value: name},
	}
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, name)
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}

}

func (c *Collector) removeSourceCharts(name string) {
	px := fmt.Sprintf("icecast_%s_", cleanSource(name))
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func cleanSource(name string) string {
	r := strings.NewReplacer(" ", "_", ".", "_", ",", "_")
	return r.Replace(name)
}
