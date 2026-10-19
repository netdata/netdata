// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
)

const c020RawSourceUnitsContract = "CASE-020/raw-source-units-after-grouping"
const c020CombinedViewUnitsContract = "CASE-020/combined-view-units"

func c020AssertCombinedViewUnits(t *testing.T, doc map[string]any, expected ...string) {
	t.Helper()
	view := queryObject(t, doc, "view", "view")
	if len(expected) == 1 {
		if got, ok := view["units"].(string); !ok || got != expected[0] {
			t.Errorf("view.units = %v, want common result unit %q", view["units"], expected[0])
		}
		return
	}
	units, ok := view["units"].([]any)
	if !ok || len(units) != len(expected) {
		t.Fatalf("view.units = %v, want distinct result units %v", view["units"], expected)
	}
	got := make(map[string]bool)
	for _, value := range units {
		unit, ok := value.(string)
		if !ok || got[unit] {
			t.Fatalf("view.units has malformed or duplicate units: %v", units)
		}
		got[unit] = true
	}
	want := make(map[string]bool)
	for _, unit := range expected {
		want[unit] = true
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("view.units = %v, want distinct result units %v", units, expected)
	}
}

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

func c020AssertGroupedRawUnits(t *testing.T, doc map[string]any, firstUnits string) {
	t.Helper()
	db := queryObject(t, doc, "db", "db")
	dbDimensions := queryObject(t, db, "dimensions", "db.dimensions")
	view := queryObject(t, doc, "view", "view")
	viewDimensions := queryObject(t, view, "dimensions", "view.dimensions")
	ids, idsOK := dbDimensions["ids"].([]any)
	viewIDs, viewIDsOK := viewDimensions["ids"].([]any)
	counts, countsOK := viewDimensions["aggregated"].([]any)
	if !idsOK || !viewIDsOK || !countsOK || len(ids) < 1 || len(ids) > 2 ||
		len(viewIDs) != len(ids) || len(counts) != len(ids) {
		t.Fatalf("cannot establish raw-unit group membership: db=%v view=%v", dbDimensions, viewDimensions)
	}
	// Resolve source ownership before inspecting units; normalization has its own verdict.
	want := make(map[string]string)
	for i, value := range ids {
		id, idOK := value.(string)
		viewID, viewIDOK := viewIDs[i].(string)
		count, numeric := queryInteger(counts[i])
		if !idOK || id == "" || !viewIDOK || viewID != id || !numeric || want[id] != "" {
			t.Fatalf("cannot establish raw-unit group membership: db=%v view=%v", dbDimensions, viewDimensions)
		}
		if len(ids) == 1 && count == 2 {
			want[id] = firstUnits
		} else if len(ids) == 2 && count == 1 && (id == "requests/s" || id == "requests") {
			want[id] = id
		} else {
			t.Fatalf("unexpected raw-unit group membership: id=%q contributors=%d groups=%d", id, count, len(ids))
		}
	}
	c020AssertDimensionUnits(t, db, "db", want)
}

func c020AssertFoldedRawUnits(t *testing.T, doc map[string]any) {
	t.Helper()
	db := queryObject(t, doc, "db", "db")
	dimensions := queryObject(t, db, "dimensions", "db.dimensions")
	ids, ok := dimensions["ids"].([]any)
	if !ok || len(ids) != 2 {
		t.Fatalf("fold must retain two raw-unit entries: %v", dimensions)
	}
	want := make(map[string]string)
	for _, value := range ids {
		id, idOK := value.(string)
		if !idOK || id == "" || want[id] != "" {
			t.Fatalf("fold has malformed or duplicate raw-unit IDs: %v", ids)
		}
		want[id] = "requests/s"
	}
	c020AssertDimensionUnits(t, db, "db", want)
}

func TestCase020NormalizedUnitGroups(t *testing.T) {
	const contract = "CASE-020/normalized-unit-groups"
	for index, name := range []string{"rate-first", "gauge-first"} {
		t.Run(name, func(t *testing.T) {
			registerContractComponent(t, contract, name)
			registerContractComponent(t, c020RawSourceUnitsContract, "normalized/"+name)
			registerContractComponent(t, c020CombinedViewUnitsContract, "normalized/"+name)
			host := "c020-unit-order-" + name
			context := "fixture.c020_unit_order_" + strconv.Itoa(index)
			rate := c020Chart(context+"_z", context+"_rate", "requests/s", "incremental", 5, fixture.T0)
			gauge := c020Chart(context+"_a", context+"_gauge", "requests", "absolute", 7, fixture.T0)
			charts := []fixture.Chart{rate, gauge}
			if name == "gauge-first" {
				charts[0], charts[1] = charts[1], charts[0]
			}
			c020PushCharts(t, host, guid(9995+index), charts...)
			waitChartsRetention(t, host, charts...)
			doc := c020Query(t, c020QuerySpec{
				host: host, context: context + "*", timeGroup: "sum", groupBy: "units", aggregation: "sum",
				after: fixture.T0, before: fixture.T0 + c020Samples,
			})
			t.Run("result-units", func(t *testing.T) {
				trackContractComponent(t, contract, name)
				label, _ := c020OneColumn(t, doc)
				if label != "requests" {
					t.Errorf("normalized units group = %q, want requests", label)
				}
				view := queryObject(t, doc, "view", "view")
				c020AssertDimensionUnits(t, view, "view", map[string]string{"requests": "requests"})
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
			t.Run("db-units", func(t *testing.T) {
				trackContractComponent(t, c020RawSourceUnitsContract, "normalized/"+name)
				c020AssertGroupedRawUnits(t, doc, charts[0].Units)
			})
			t.Run("combined-view-units", func(t *testing.T) {
				trackContractComponent(t, c020CombinedViewUnitsContract, "normalized/"+name)
				c020AssertCombinedViewUnits(t, doc, "requests")
			})
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
					component := item.name + "/" + groupBy
					registerContractComponent(t, contract, component)
					registerContractComponent(t, c020RawSourceUnitsContract, "folded/"+component)
					registerContractComponent(t, c020CombinedViewUnitsContract, "folded/"+component)
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
					t.Run("db-units", func(t *testing.T) {
						trackContractComponent(t, c020RawSourceUnitsContract, "folded/"+component)
						c020AssertFoldedRawUnits(t, doc)
					})
					t.Run("result-units", func(t *testing.T) {
						trackContractComponent(t, contract, component)
						c020AssertDimensionUnits(t, queryObject(t, doc, "view", "view"), "view", map[string]string{
							keptID: item.keptUnits, "remaining 2 dimensions": item.foldUnits,
						})
					})
					t.Run("combined-view-units", func(t *testing.T) {
						trackContractComponent(t, c020CombinedViewUnitsContract, "folded/"+component)
						if item.keptUnits == item.foldUnits {
							c020AssertCombinedViewUnits(t, doc, item.keptUnits)
						} else {
							c020AssertCombinedViewUnits(t, doc, item.keptUnits, item.foldUnits)
						}
					})
				})
			}
		})
	}
	t.Run("equal-normalized", func(t *testing.T) {
		registerContractComponent(t, contract, "equal-normalized")
		registerContractComponent(t, c020RawSourceUnitsContract, "folded/equal-normalized")
		registerContractComponent(t, c020CombinedViewUnitsContract, "folded/equal-normalized")
		const host = "c020-fold-equal"
		const context = "fixture.c020_fold_equal"
		var charts []fixture.Chart
		for i := 1; i <= 3; i++ {
			units, algorithm, sourceContext := "requests/s", "incremental", context+"_rate"
			if i == 1 {
				units, algorithm, sourceContext = "requests", "absolute", context+"_gauge"
			}
			chart := c020Chart(context+"_"+strconv.Itoa(i), sourceContext, units, algorithm, i, fixture.T0)
			chart.Dimensions[0].ID = "d" + strconv.Itoa(i)
			charts = append(charts, chart)
		}
		c020PushCharts(t, host, guid(9994), charts...)
		waitChartsRetention(t, host, charts...)
		params := daemon.DataParams(context+"*", fixture.T0, fixture.T0+c020Samples, 1)
		params.Set("time_group", "sum")
		params.Set("group_by", "dimension,units")
		params.Set("aggregation", "sum")
		params.Set("options", "jsonwrap|unaligned")
		params.Set("cardinality_limit", "2")
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		t.Run("db-units", func(t *testing.T) {
			trackContractComponent(t, c020RawSourceUnitsContract, "folded/equal-normalized")
			c020AssertFoldedRawUnits(t, doc)
		})
		t.Run("result-units", func(t *testing.T) {
			trackContractComponent(t, contract, "equal-normalized")
			c020AssertDimensionUnits(t, queryObject(t, doc, "view", "view"), "view", map[string]string{
				"d3,requests": "requests", "remaining 2 dimensions": "requests",
			})
		})
		t.Run("combined-view-units", func(t *testing.T) {
			trackContractComponent(t, c020CombinedViewUnitsContract, "folded/equal-normalized")
			c020AssertCombinedViewUnits(t, doc, "requests")
		})
	})

}
