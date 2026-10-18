// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
)

func c020AssertDimensionUnits(t *testing.T, section map[string]any, path string, want map[string]string) {
	t.Helper()
	dimensions := queryObject(t, section, "dimensions", path+".dimensions")
	ids, idsOK := dimensions["ids"].([]any)
	units, unitsOK := dimensions["units"].([]any)
	if !idsOK || !unitsOK || len(ids) != len(want) || len(units) != len(ids) {
		t.Fatalf("%s has malformed or incomplete dimension units: %v", path, dimensions)
	}
	seen := make(map[string]bool)
	for i, value := range ids {
		id, idOK := value.(string)
		unit, unitOK := units[i].(string)
		expected, exists := want[id]
		if !idOK || !unitOK || !exists || seen[id] || unit != expected {
			t.Errorf("%s dimension %v units = %v, want %v", path, value, units[i], want)
		}
		seen[id] = true
	}
}

func c020WaitCharts(t *testing.T, host string, charts ...fixture.Chart) {
	t.Helper()
	// Context retention alone may be satisfied before its second chart arrives.
	for _, chart := range charts {
		params := daemon.DataParams(chart.Context, chart.FirstT()-1, chart.LastT(), c020Samples)
		params.Set("instances", chart.ID)
		deadline := time.Now().Add(15 * time.Second)
		for {
			doc, err := td.DataV3(host, params)
			if err == nil {
				retention, ok := daemon.QueryRetention(doc)
				if ok && retention.FirstEntry == chart.FirstT() && retention.LastEntry == chart.LastT() {
					break
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("chart %q did not finish ingestion", chart.ID)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func TestCase020NormalizedUnitGroups(t *testing.T) {
	const contract = "CASE-020/normalized-unit-groups"
	for index, name := range []string{"rate-first", "gauge-first"} {
		t.Run(name, func(t *testing.T) {
			trackContractComponent(t, contract, name)
			host := "c020-unit-order-" + name
			context := "fixture.c020_unit_order_" + strconv.Itoa(index)
			rate := c020Chart(context+"_z", context, "requests/s", "incremental", 5, fixture.T0)
			gauge := c020Chart(context+"_a", context, "requests", "absolute", 7, fixture.T0)
			charts := []fixture.Chart{rate, gauge}
			if name == "gauge-first" {
				charts[0], charts[1] = charts[1], charts[0]
			}
			c020PushCharts(t, host, guid(9995+index), charts...)
			c020WaitCharts(t, host, charts...)
			doc := c020Query(t, c020QuerySpec{
				host: host, context: context, timeGroup: "sum", groupBy: "units", aggregation: "sum",
				after: fixture.T0, before: fixture.T0 + c020Samples,
			})
			label, _ := c020OneColumn(t, doc)
			if label != "requests" {
				t.Errorf("normalized units group = %q, want requests", label)
			}
			view := queryObject(t, doc, "view", "view")
			c020AssertDimensionUnits(t, view, "view", map[string]string{"requests": "requests"})
			c020AssertDimensionUnits(t, queryObject(t, doc, "db", "db"), "db", map[string]string{
				"requests": charts[0].Units,
			})
			dimensions := queryObject(t, view, "dimensions", "view.dimensions")
			counts, ok := dimensions["aggregated"].([]any)
			if !ok || len(counts) != 1 {
				t.Fatalf("units group has invalid contributor counts: %v", dimensions["aggregated"])
			}
			count, numeric := queryInteger(counts[0])
			if !numeric || count != 2 {
				t.Errorf("units group did not retain both contributors: %v", dimensions["aggregated"])
			}
		})
	}
}

func TestCase020FoldedDimensionUnits(t *testing.T) {
	const contract = "CASE-020/folded-dimension-units"
	for index, item := range []struct {
		name       string
		algorithms [3]string
		keptUnits  string
		foldUnits  string
	}{
		{"gauges", [3]string{"absolute", "absolute", "absolute"}, "requests/s", "requests/s"},
		{"rates", [3]string{"incremental", "incremental", "incremental"}, "requests", "requests"},
		{"mixed", [3]string{"incremental", "absolute", "incremental"}, "requests", "requests/s"},
		{"mixed-rate-first", [3]string{"absolute", "incremental", "incremental"}, "requests", "requests/s"},
	} {
		t.Run(item.name, func(t *testing.T) {
			host := "c020-fold-" + item.name
			context := "fixture.c020_fold_" + item.name
			chart := c020Chart(context, context, "requests/s", item.algorithms[0], 1, fixture.T0)
			chart.Dimensions = nil
			for i, algorithm := range item.algorithms {
				dimension := c020Chart(context, context, "requests/s", algorithm, i+1, fixture.T0).Dimensions[0]
				dimension.ID = "d" + strconv.Itoa(i+1)
				chart.Dimensions = append(chart.Dimensions, dimension)
			}
			c020PushCharts(t, host, guid(9990+index), chart)
			for _, groupBy := range []string{"dimension", "dimension,units"} {
				t.Run(groupBy, func(t *testing.T) {
					trackContractComponent(t, contract, item.name+"/"+groupBy)
					params := daemon.DataParams(context, fixture.T0, fixture.T0+c020Samples, 1)
					params.Set("time_group", "sum")
					params.Set("group_by", groupBy)
					params.Set("aggregation", "sum")
					params.Set("options", "jsonwrap|unaligned")
					params.Set("cardinality_limit", "2")
					doc, err := td.DataV3(host, params)
					if err != nil {
						t.Fatal(err)
					}
					keptID := "d3"
					if groupBy == "dimension,units" {
						keptID += "," + item.keptUnits
					}
					c020AssertDimensionUnits(t, queryObject(t, doc, "db", "db"), "db", map[string]string{
						keptID: "requests/s", "remaining 2 dimensions": "requests/s",
					})
					c020AssertDimensionUnits(t, queryObject(t, doc, "view", "view"), "view", map[string]string{
						keptID: item.keptUnits, "remaining 2 dimensions": item.foldUnits,
					})
				})
			}
		})
	}
	t.Run("equal-normalized", func(t *testing.T) {
		trackContractComponent(t, contract, "equal-normalized")
		const host = "c020-fold-equal"
		const context = "fixture.c020_fold_equal"
		var charts []fixture.Chart
		for i := 1; i <= 3; i++ {
			units, algorithm := "requests/s", "incremental"
			if i == 1 {
				units, algorithm = "requests", "absolute"
			}
			chart := c020Chart(context+"_"+strconv.Itoa(i), context, units, algorithm, i, fixture.T0)
			chart.Dimensions[0].ID = "d" + strconv.Itoa(i)
			charts = append(charts, chart)
		}
		c020PushCharts(t, host, guid(9994), charts...)
		c020WaitCharts(t, host, charts...)
		params := daemon.DataParams(context, fixture.T0, fixture.T0+c020Samples, 1)
		params.Set("time_group", "sum")
		params.Set("group_by", "dimension,units")
		params.Set("aggregation", "sum")
		params.Set("options", "jsonwrap|unaligned")
		params.Set("cardinality_limit", "2")
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		c020AssertDimensionUnits(t, queryObject(t, doc, "db", "db"), "db", map[string]string{
			"d3,requests": "requests/s", "remaining 2 dimensions": "requests/s",
		})
		c020AssertDimensionUnits(t, queryObject(t, doc, "view", "view"), "view", map[string]string{
			"d3,requests": "requests", "remaining 2 dimensions": "requests",
		})
	})

}
