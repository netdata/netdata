// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

var familyCorrectnessChartTmpl = Chart{
	ID:       "family_%s_correctness",
	Title:    "BGP route correctness",
	Units:    "routes",
	Fam:      "%s",
	Ctx:      "bgp.family_correctness",
	Type:     collectorapi.Stacked,
	Priority: prioFamilyCorrectness,
	Dims: Dims{
		{ID: "family_%s_correctness_valid", Name: "valid"},
		{ID: "family_%s_correctness_invalid", Name: "invalid"},
		{ID: "family_%s_correctness_not_found", Name: "not_found"},
	},
}

func (c *Collector) addFamilyCorrectnessChart(f familyStats) {
	id := familyCorrectnessChartID(f.ID)
	if c.Charts().Has(id) {
		return
	}

	chart := familyCorrectnessChartTmpl.Copy()
	chart.ID = fmt.Sprintf(chart.ID, f.ID)
	chart.Fam = fmt.Sprintf(chart.Fam, familyDisplay(f))
	chart.Labels = familyLabels(f)
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, f.ID)
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func familyCorrectnessChartID(familyID string) string {
	return fmt.Sprintf(familyCorrectnessChartTmpl.ID, familyID)
}
