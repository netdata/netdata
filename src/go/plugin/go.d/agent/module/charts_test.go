// SPDX-License-Identifier: GPL-3.0-or-later

package module

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestChart(id string) *Chart {
	return &Chart{
		ID:    id,
		Title: "Title",
		Units: "units",
		Fam:   "family",
		Ctx:   "context",
		Type:  Line,
		Dims: Dims{
			{ID: "dim1", Algo: Absolute},
		},
		Vars: Vars{
			{ID: "var1", Value: 1},
		},
	}
}

func TestDimAlgo_String(t *testing.T) {
	cases := []struct {
		expected string
		actual   fmt.Stringer
	}{
		{"absolute", Absolute},
		{"incremental", Incremental},
		{"percentage-of-absolute-row", PercentOfAbsolute},
		{"percentage-of-incremental-row", PercentOfIncremental},
		{"absolute", DimAlgo("wrong")},
	}

	for _, v := range cases {
		assert.Equal(t, v.expected, v.actual.String())
	}
}

func TestChartType_String(t *testing.T) {
	cases := []struct {
		expected string
		actual   fmt.Stringer
	}{
		{"line", Line},
		{"area", Area},
		{"stacked", Stacked},
		{"line", ChartType("wrong")},
	}

	for _, v := range cases {
		assert.Equal(t, v.expected, v.actual.String())
	}
}

func TestOpts_String(t *testing.T) {
	cases := []struct {
		expected string
		actual   fmt.Stringer
	}{
		{"", Opts{}},
		{
			"detail hidden obsolete store_first",
			Opts{Detail: true, Hidden: true, Obsolete: true, StoreFirst: true},
		},
		{
			"detail hidden obsolete store_first",
			Opts{Detail: true, Hidden: true, Obsolete: true, StoreFirst: true},
		},
	}

	for _, v := range cases {
		assert.Equal(t, v.expected, v.actual.String())
	}
}

func TestDimOpts_String(t *testing.T) {
	cases := []struct {
		expected string
		actual   fmt.Stringer
	}{
		{"", DimOpts{}},
		{
			"hidden nooverflow noreset obsolete type=float",
			DimOpts{Hidden: true, NoOverflow: true, NoReset: true, Obsolete: true, Float: true},
		},
		{
			"hidden obsolete",
			DimOpts{Hidden: true, NoOverflow: false, NoReset: false, Obsolete: true},
		},
	}

	for _, v := range cases {
		assert.Equal(t, v.expected, v.actual.String())
	}
}

func TestCharts_Copy(t *testing.T) {
	orig := &Charts{
		createTestChart("1"),
		createTestChart("2"),
	}
	copied := orig.Copy()

	require.False(t, orig == copied, "Charts copy points to the same address")
	require.Len(t, *orig, len(*copied))

	for idx := range *orig {
		compareCharts(t, (*orig)[idx], (*copied)[idx])
	}
}

func TestChart_Copy(t *testing.T) {
	orig := createTestChart("1")

	compareCharts(t, orig, orig.Copy())
}

func TestCharts_Add(t *testing.T) {
	charts := Charts{}
	chart1 := createTestChart("1")
	chart2 := createTestChart("2")
	chart3 := createTestChart("")

	// OK case
	assert.NoError(t, charts.Add(
		chart1,
		chart2,
	))
	assert.Len(t, charts, 2)

	// NG case
	assert.Error(t, charts.Add(
		chart3,
		chart1,
		chart2,
	))
	assert.Len(t, charts, 2)

	assert.True(t, charts[0] == chart1)
	assert.True(t, charts[1] == chart2)
}

func TestCharts_Add_SameID(t *testing.T) {
	charts := Charts{}
	chart1 := createTestChart("1")
	chart2 := createTestChart("1")

	assert.NoError(t, charts.Add(chart1))
	assert.Error(t, charts.Add(chart2))
	assert.Len(t, charts, 1)

	charts = Charts{}
	chart1 = createTestChart("1")
	chart2 = createTestChart("1")

	assert.NoError(t, charts.Add(chart1))
	chart1.MarkRemove()
	assert.NoError(t, charts.Add(chart2))
	assert.Len(t, charts, 2)
}

func TestCharts_Get(t *testing.T) {
	chart := createTestChart("1")
	charts := Charts{
		chart,
	}

	// OK case
	assert.True(t, chart == charts.Get("1"))
	// NG case
	assert.Nil(t, charts.Get("2"))
}

func TestCharts_Has(t *testing.T) {
	chart := createTestChart("1")
	charts := &Charts{
		chart,
	}

	// OK case
	assert.True(t, charts.Has("1"))
	// NG case
	assert.False(t, charts.Has("2"))
}

func TestCharts_Remove(t *testing.T) {
	chart := createTestChart("1")
	charts := &Charts{
		chart,
	}

	// OK case
	assert.NoError(t, charts.Remove("1"))
	assert.Len(t, *charts, 0)

	// NG case
	assert.Error(t, charts.Remove("2"))
}

func TestChart_AddDim(t *testing.T) {
	chart := createTestChart("1")
	dim := &Dim{ID: "dim2"}

	// OK case
	assert.NoError(t, chart.AddDim(dim))
	assert.Len(t, chart.Dims, 2)

	// NG case
	assert.Error(t, chart.AddDim(dim))
	assert.Len(t, chart.Dims, 2)
}

func TestChart_AddVar(t *testing.T) {
	chart := createTestChart("1")
	variable := &Var{ID: "var2"}

	// OK case
	assert.NoError(t, chart.AddVar(variable))
	assert.Len(t, chart.Vars, 2)

	// NG case
	assert.Error(t, chart.AddVar(variable))
	assert.Len(t, chart.Vars, 2)
}

func TestChart_GetDim(t *testing.T) {
	chart := &Chart{
		Dims: Dims{
			{ID: "1"},
			{ID: "2"},
		},
	}

	// OK case
	assert.True(t, chart.GetDim("1") != nil && chart.GetDim("1").ID == "1")

	// NG case
	assert.Nil(t, chart.GetDim("3"))
}

func TestChart_RemoveDim(t *testing.T) {
	chart := createTestChart("1")

	// OK case
	assert.NoError(t, chart.RemoveDim("dim1"))
	assert.Len(t, chart.Dims, 0)

	// NG case
	assert.Error(t, chart.RemoveDim("dim2"))
}

func TestChart_HasDim(t *testing.T) {
	chart := createTestChart("1")

	// OK case
	assert.True(t, chart.HasDim("dim1"))
	// NG case
	assert.False(t, chart.HasDim("dim2"))
}

func TestChart_MarkNotCreated(t *testing.T) {
	chart := createTestChart("1")

	chart.MarkNotCreated()
	assert.False(t, chart.created)
}

func TestChart_MarkRemove(t *testing.T) {
	chart := createTestChart("1")

	chart.MarkRemove()
	assert.True(t, chart.remove)
	assert.True(t, chart.Obsolete)
}

func TestChart_MarkDimRemove(t *testing.T) {
	chart := createTestChart("1")

	assert.Error(t, chart.MarkDimRemove("dim99", false))
	assert.NoError(t, chart.MarkDimRemove("dim1", true))
	assert.True(t, chart.GetDim("dim1").Obsolete)
	assert.True(t, chart.GetDim("dim1").Hidden)
	assert.True(t, chart.GetDim("dim1").remove)
}

func TestChart_check(t *testing.T) {
	// OK case
	chart := createTestChart("1")
	assert.NoError(t, checkChart(chart))

	// NG case
	chart = createTestChart("1")
	chart.ID = ""
	assert.Error(t, checkChart(chart))

	chart = createTestChart("1")
	chart.ID = "invalid id"
	assert.Error(t, checkChart(chart))

	chart = createTestChart("1")
	chart.Title = ""
	assert.Error(t, checkChart(chart))

	chart = createTestChart("1")
	chart.Units = ""
	assert.Error(t, checkChart(chart))

	chart = createTestChart("1")
	chart.Dims = Dims{
		{ID: "1"},
		{ID: "1"},
	}
	assert.Error(t, checkChart(chart))

	chart = createTestChart("1")
	chart.Vars = Vars{
		{ID: "1"},
		{ID: "1"},
	}
	assert.Error(t, checkChart(chart))
}

func TestDim_check(t *testing.T) {
	// OK case
	dim := &Dim{ID: "id"}
	assert.NoError(t, checkDim(dim))

	// NG case
	dim = &Dim{ID: "id"}
	dim.ID = ""
	assert.Error(t, checkDim(dim))

	dim = &Dim{ID: "id"}
	dim.ID = "invalid id"
	assert.Error(t, checkDim(dim))

	dim = &Dim{ID: "i d", Name: "id"}
	assert.NoError(t, checkDim(dim))
}

func TestVar_check(t *testing.T) {
	// OK case
	v := &Var{ID: "id"}
	assert.NoError(t, checkVar(v))

	// NG case
	v = &Var{ID: "id"}
	v.ID = ""
	assert.Error(t, checkVar(v))

	v = &Var{ID: "id"}
	v.ID = "invalid id"
	assert.Error(t, checkVar(v))
}

func compareCharts(t *testing.T, orig, copied *Chart) {
	// 1. compare chart pointers
	// 2. compare Dims, Vars length
	// 3. compare Dims, Vars pointers

	assert.False(t, orig == copied, "Chart copy ChartsFunc points to the same address")

	require.Len(t, orig.Dims, len(copied.Dims))
	require.Len(t, orig.Vars, len(copied.Vars))

	for idx := range (*orig).Dims {
		assert.False(t, orig.Dims[idx] == copied.Dims[idx], "Chart copy dim points to the same address")
		assert.Equal(t, orig.Dims[idx], copied.Dims[idx], "Chart copy dim isn't equal to orig")
	}

	for idx := range (*orig).Vars {
		assert.False(t, orig.Vars[idx] == copied.Vars[idx], "Chart copy var points to the same address")
		assert.Equal(t, orig.Vars[idx], copied.Vars[idx], "Chart copy var isn't equal to orig")
	}
}
