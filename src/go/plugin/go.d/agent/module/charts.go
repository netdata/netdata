// SPDX-License-Identifier: GPL-3.0-or-later

package module

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
)

type (
	ChartType string
	DimAlgo   string
)

const (
	// Line chart type.
	Line ChartType = "line"
	// Area chart type.
	Area ChartType = "area"
	// Stacked chart type.
	Stacked ChartType = "stacked"

	// Absolute dimension algorithm.
	// The value is to drawn as-is (interpolated to second boundary).
	Absolute DimAlgo = "absolute"
	// Incremental dimension algorithm.
	// The value increases over time, the difference from the last value is presented in the chart,
	// the server interpolates the value and calculates a per second figure.
	Incremental DimAlgo = "incremental"
	// PercentOfAbsolute dimension algorithm.
	// The percent of this value compared to the total of all dimensions.
	PercentOfAbsolute DimAlgo = "percentage-of-absolute-row"
	// PercentOfIncremental dimension algorithm.
	// The percent of this value compared to the incremental total of all dimensions
	PercentOfIncremental DimAlgo = "percentage-of-incremental-row"
)

const (
	// Not documented.
	// https://github.com/netdata/netdata/blob/cc2586de697702f86a3c34e60e23652dd4ddcb42/database/rrd.h#L204

	LabelSourceAuto = 1 << 0
	LabelSourceConf = 1 << 1
	LabelSourceK8s  = 1 << 2
)

func (d DimAlgo) String() string {
	switch d {
	case Absolute, Incremental, PercentOfAbsolute, PercentOfIncremental:
		return string(d)
	}
	return string(Absolute)
}

func (c ChartType) String() string {
	switch c {
	case Line, Area, Stacked:
		return string(c)
	}
	return string(Line)
}

type (
	// Charts is a collection of Charts.
	Charts []*Chart

	// Opts represents chart options.
	Opts struct {
		Obsolete   bool
		Detail     bool
		StoreFirst bool
		Hidden     bool
	}

	// Chart represents a chart.
	// For the full description please visit https://docs.netdata.cloud/plugins.d/#chart
	Chart struct {
		// typeID is the unique identification of the chart, if not specified,
		// the orchestrator will use job full name + chart ID as typeID (default behaviour).
		typ string
		id  string

		OverModule   string
		TypeOverride string
		IDSep        bool
		ID           string
		OverID       string
		Title        string
		Units        string
		Fam          string
		Ctx          string
		Type         ChartType
		Priority     int
		UpdateEvery  int  // Override for this chart's update interval (0 means use job default)
		SkipGaps     bool // Skip chart entirely (no BEGIN/END) when no dimensions have data
		Opts

		Labels []Label
		Dims   Dims
		Vars   Vars

		Retries int

		remove bool
		// created flag is used to indicate whether the chart needs to be created by the orchestrator.
		created bool
		// updated flag is used to indicate whether the chart was updated on last data collection interval.
		updated bool

		// ignore flag is used to indicate that the chart shouldn't be sent to the netdata plugins.d
		ignore bool
	}

	Label struct {
		Key    string
		Value  string
		Source int
	}

	// DimOpts represents dimension options.
	DimOpts struct {
		Obsolete   bool
		Hidden     bool
		NoReset    bool
		NoOverflow bool
	}

	// Dim represents a chart dimension.
	// For detailed description please visit https://docs.netdata.cloud/plugins.d/#dimension.
	Dim struct {
		ID   string
		Name string
		Algo DimAlgo
		Mul  int
		Div  int
		DimOpts

		remove bool
	}

	// Var represents a chart variable.
	// For detailed description please visit https://docs.netdata.cloud/plugins.d/#variable
	Var struct {
		ID    string
		Name  string
		Value int64
	}

	// Dims is a collection of dims.
	Dims []*Dim
	// Vars is a collection of vars.
	Vars []*Var
)

func (o Opts) String() string {
	var b strings.Builder
	if o.Detail {
		b.WriteString(" detail")
	}
	if o.Hidden {
		b.WriteString(" hidden")
	}
	if o.Obsolete {
		b.WriteString(" obsolete")
	}
	if o.StoreFirst {
		b.WriteString(" store_first")
	}

	if len(b.String()) == 0 {
		return ""
	}
	return b.String()[1:]
}

func (o DimOpts) String() string {
	var b strings.Builder
	if o.Hidden {
		b.WriteString(" hidden")
	}
	if o.NoOverflow {
		b.WriteString(" nooverflow")
	}
	if o.NoReset {
		b.WriteString(" noreset")
	}
	if o.Obsolete {
		b.WriteString(" obsolete")
	}

	if len(b.String()) == 0 {
		return ""
	}
	return b.String()[1:]
}

// Add adds (appends) a variable number of Charts.
func (c *Charts) Add(charts ...*Chart) error {
	for _, chart := range charts {
		err := checkChart(chart)
		if err != nil {
			return fmt.Errorf("error on adding chart '%s' : %s", chart.ID, err)
		}
		if chart := c.Get(chart.ID); chart != nil && !chart.remove {
			return fmt.Errorf("error on adding chart : '%s' is already in charts", chart.ID)
		}
		*c = append(*c, chart)
	}

	return nil
}

// Get returns the chart by ID.
func (c Charts) Get(chartID string) *Chart {
	idx := c.index(chartID)
	if idx == -1 {
		return nil
	}
	return c[idx]
}

// Has returns true if ChartsFunc contain the chart with the given ID, false otherwise.
func (c Charts) Has(chartID string) bool {
	return c.index(chartID) != -1
}

// Remove removes the chart from Charts by ID.
// Avoid to use it in runtime.
func (c *Charts) Remove(chartID string) error {
	idx := c.index(chartID)
	if idx == -1 {
		return fmt.Errorf("error on removing chart : '%s' is not in charts", chartID)
	}
	copy((*c)[idx:], (*c)[idx+1:])
	(*c)[len(*c)-1] = nil
	*c = (*c)[:len(*c)-1]
	return nil
}

// Copy returns a deep copy of ChartsFunc.
func (c Charts) Copy() *Charts {
	charts := Charts{}
	for idx := range c {
		charts = append(charts, c[idx].Copy())
	}
	return &charts
}

func (c Charts) index(chartID string) int {
	for idx := range c {
		if c[idx].ID == chartID {
			return idx
		}
	}
	return -1
}

// MarkNotCreated changes 'created' chart flag to false.
// Use it to add dimension in runtime.
func (c *Chart) MarkNotCreated() {
	c.created = false
}

// MarkRemove sets 'remove' flag and Obsolete option to true.
// Use it to remove chart in runtime.
func (c *Chart) MarkRemove() {
	c.Obsolete = true
	c.remove = true
}

// MarkDimRemove sets 'remove' flag, Obsolete and optionally Hidden options to true.
// Use it to remove dimension in runtime.
func (c *Chart) MarkDimRemove(dimID string, hide bool) error {
	if !c.HasDim(dimID) {
		return fmt.Errorf("chart '%s' has no '%s' dimension", c.ID, dimID)
	}
	dim := c.GetDim(dimID)
	dim.Obsolete = true
	if hide {
		dim.Hidden = true
	}
	dim.remove = true
	return nil
}

// AddDim adds new dimension to the chart dimensions.
func (c *Chart) AddDim(newDim *Dim) error {
	err := checkDim(newDim)
	if err != nil {
		return fmt.Errorf("error on adding dim to chart '%s' : %s", c.ID, err)
	}
	if c.HasDim(newDim.ID) {
		return fmt.Errorf("error on adding dim : '%s' is already in chart '%s' dims", newDim.ID, c.ID)
	}
	c.Dims = append(c.Dims, newDim)

	return nil
}

// AddVar adds new variable to the chart variables.
func (c *Chart) AddVar(newVar *Var) error {
	err := checkVar(newVar)
	if err != nil {
		return fmt.Errorf("error on adding var to chart '%s' : %s", c.ID, err)
	}
	if c.indexVar(newVar.ID) != -1 {
		return fmt.Errorf("error on adding var : '%s' is already in chart '%s' vars", newVar.ID, c.ID)
	}
	c.Vars = append(c.Vars, newVar)

	return nil
}

// GetDim returns dimension by ID.
func (c *Chart) GetDim(dimID string) *Dim {
	idx := c.indexDim(dimID)
	if idx == -1 {
		return nil
	}
	return c.Dims[idx]
}

// RemoveDim removes dimension by ID.
// Avoid to use it in runtime.
func (c *Chart) RemoveDim(dimID string) error {
	idx := c.indexDim(dimID)
	if idx == -1 {
		return fmt.Errorf("error on removing dim : '%s' isn't in chart '%s'", dimID, c.ID)
	}
	c.Dims = append(c.Dims[:idx], c.Dims[idx+1:]...)

	return nil
}

// HasDim returns true if the chart contains dimension with the given ID, false otherwise.
func (c Chart) HasDim(dimID string) bool {
	return c.indexDim(dimID) != -1
}

// Copy returns a deep copy of the chart.
func (c Chart) Copy() *Chart {
	chart := c
	chart.Dims = Dims{}
	chart.Vars = Vars{}

	for idx := range c.Dims {
		chart.Dims = append(chart.Dims, c.Dims[idx].copy())
	}
	for idx := range c.Vars {
		chart.Vars = append(chart.Vars, c.Vars[idx].copy())
	}

	return &chart
}

func (c Chart) indexDim(dimID string) int {
	for idx := range c.Dims {
		if c.Dims[idx].ID == dimID {
			return idx
		}
	}
	return -1
}

func (c Chart) indexVar(varID string) int {
	for idx := range c.Vars {
		if c.Vars[idx].ID == varID {
			return idx
		}
	}
	return -1
}

func (d Dim) copy() *Dim {
	return &d
}

func (v Var) copy() *Var {
	return &v
}

func checkCharts(charts ...*Chart) error {
	for _, chart := range charts {
		err := checkChart(chart)
		if err != nil {
			return fmt.Errorf("chart '%s' : %v", chart.ID, err)
		}
	}
	return nil
}

func checkChart(chart *Chart) error {
	if chart.ID == "" {
		return errors.New("empty ID")
	}

	if chart.Title == "" {
		return errors.New("empty Title")
	}

	if chart.Units == "" {
		return errors.New("empty Units")
	}

	if id := checkID(chart.ID); id != -1 {
		return fmt.Errorf("unacceptable symbol in ID : '%c'", id)
	}

	set := make(map[string]bool)

	for _, d := range chart.Dims {
		err := checkDim(d)
		if err != nil {
			return err
		}
		if set[d.ID] {
			return fmt.Errorf("duplicate dim '%s'", d.ID)
		}
		set[d.ID] = true
	}

	set = make(map[string]bool)

	for _, v := range chart.Vars {
		if err := checkVar(v); err != nil {
			return err
		}
		if set[v.ID] {
			return fmt.Errorf("duplicate var '%s'", v.ID)
		}
		set[v.ID] = true
	}
	return nil
}

func checkDim(d *Dim) error {
	if d.ID == "" {
		return errors.New("empty dim ID")
	}
	if id := checkID(d.ID); id != -1 && (d.Name == "" || checkID(d.Name) != -1) {
		return fmt.Errorf("unacceptable symbol in dim ID '%s' : '%c'", d.ID, id)
	}
	return nil
}

func checkVar(v *Var) error {
	if v.ID == "" {
		return errors.New("empty var ID")
	}
	if id := checkID(v.ID); id != -1 {
		return fmt.Errorf("unacceptable symbol in var ID '%s' : '%c'", v.ID, id)
	}
	return nil
}

func checkID(id string) int {
	for _, r := range id {
		if unicode.IsSpace(r) {
			return int(r)
		}
	}
	return -1
}

func TestMetricsHasAllChartsDims(t *testing.T, charts *Charts, mx map[string]int64) {
	TestMetricsHasAllChartsDimsSkip(t, charts, mx, nil)
}

func TestMetricsHasAllChartsDimsSkip(t *testing.T, charts *Charts, mx map[string]int64, skip func(chart *Chart, dim *Dim) bool) {
	for _, chart := range *charts {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			if skip != nil && skip(chart, dim) {
				continue
			}

			_, ok := mx[dim.ID]
			assert.Truef(t, ok, "missing data for dimension '%s' in chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := mx[v.ID]
			assert.Truef(t, ok, "missing data for variable '%s' in chart '%s'", v.ID, chart.ID)
		}
	}
}
