package main

// Shared types and helpers for pluginsd function JSON table responses.
// Both network-protocols (socket) and dns-queries use this schema.

type fnValueOptions struct {
	Units         string  `json:"units,omitempty"`
	Transform     string  `json:"transform"`
	DecimalPoints int     `json:"decimal_points"`
	DefaultValue  *string `json:"default_value"`
}

type fnColumnDef struct {
	Index                 int            `json:"index"`
	UniqueKey             bool           `json:"unique_key"`
	Name                  string         `json:"name"`
	Visible               bool           `json:"visible"`
	Type                  string         `json:"type"`
	Units                 string         `json:"units,omitempty"`
	Visualization         string         `json:"visualization"`
	ValueOptions          fnValueOptions `json:"value_options"`
	Sort                  string         `json:"sort"`
	Sortable              bool           `json:"sortable"`
	Sticky                bool           `json:"sticky"`
	Summary               string         `json:"summary"`
	Filter                string         `json:"filter"`
	FullWidth             bool           `json:"full_width"`
	Wrap                  bool           `json:"wrap"`
	DefaultExpandedFilter bool           `json:"default_expanded_filter"`
}

type fnChartDef struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Columns []string `json:"columns"`
}

type fnGroupByDef struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
}

type fnTableResponse struct {
	Status            int                      `json:"status"`
	Type              string                   `json:"type"`
	UpdateEvery       int                      `json:"update_every"`
	HasHistory        bool                     `json:"has_history"`
	Help              string                   `json:"help"`
	Data              [][]interface{}          `json:"data"`
	Columns           map[string]fnColumnDef   `json:"columns"`
	DefaultSortColumn string                   `json:"default_sort_column"`
	Charts            map[string]fnChartDef    `json:"charts"`
	DefaultCharts     [][]string               `json:"default_charts"`
	GroupBy           map[string]fnGroupByDef  `json:"group_by"`
	Expires           int64                    `json:"expires"`
}

func fnStrCol(idx int, name string, uniqueKey, sticky bool) fnColumnDef {
	return fnColumnDef{
		Index:         idx,
		UniqueKey:     uniqueKey,
		Name:          name,
		Visible:       true,
		Type:          "string",
		Visualization: "value",
		ValueOptions:  fnValueOptions{Transform: "none", DecimalPoints: 0},
		Sort:          "ascending",
		Sortable:      true,
		Sticky:        sticky,
		Summary:       "count",
		Filter:        "multiselect",
	}
}

func fnIntCol(idx int, name, units string) fnColumnDef {
	return fnColumnDef{
		Index:         idx,
		UniqueKey:     false,
		Name:          name,
		Visible:       true,
		Type:          "integer",
		Units:         units,
		Visualization: "value",
		ValueOptions:  fnValueOptions{Units: units, Transform: "number", DecimalPoints: 0},
		Sort:          "descending",
		Sortable:      true,
		Sticky:        false,
		Summary:       "sum",
		Filter:        "range",
	}
}
