// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

type Config struct {
	UpdateEvery        int `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`

	Driver  string           `yaml:"driver" json:"driver"`
	DSN     string           `yaml:"dsn" json:"dsn"`
	Timeout confopt.Duration `yaml:"timeout" json:"timeout"`

	StaticLabels map[string]string   `yaml:"static_labels,omitempty" json:"static_labels"`
	Queries      []ConfigQueryDef    `yaml:"queries,omitempty" json:"queries"`
	Metrics      []ConfigMetricBlock `yaml:"metrics,omitempty" json:"metrics"`
	Functions    []ConfigFunction    `yaml:"functions,omitempty" json:"functions,omitempty"`
	FunctionOnly bool                `yaml:"function_only,omitempty" json:"function_only,omitempty"`
}

const (
	defaultFunctionLimit = 100
	maxFunctionLimit     = 10000
)

type ConfigFunction struct {
	ID              string                      `yaml:"id" json:"id"`
	Name            string                      `yaml:"name,omitempty" json:"name,omitempty"`
	Description     string                      `yaml:"description,omitempty" json:"description,omitempty"`
	Query           string                      `yaml:"query" json:"query"`
	Timeout         confopt.Duration            `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Limit           int                         `yaml:"limit,omitempty" json:"limit,omitempty"`
	DefaultSort     string                      `yaml:"default_sort,omitempty" json:"default_sort,omitempty"`
	DefaultSortDesc *bool                       `yaml:"default_sort_desc,omitempty" json:"default_sort_desc,omitempty"`
	Columns         map[string]ConfigFuncColumn `yaml:"columns,omitempty" json:"columns,omitempty"`
}

type ConfigFuncColumn struct {
	Type     string `yaml:"type,omitempty" json:"type,omitempty"`
	Units    string `yaml:"units,omitempty" json:"units,omitempty"`
	Tooltip  string `yaml:"tooltip,omitempty" json:"tooltip,omitempty"`
	Visible  *bool  `yaml:"visible,omitempty" json:"visible,omitempty"`
	Sortable *bool  `yaml:"sortable,omitempty" json:"sortable,omitempty"`
}

func (f *ConfigFunction) derivedName() string {
	if f.Name != "" {
		return f.Name
	}
	return deriveNameFromID(f.ID)
}

func deriveNameFromID(id string) string {
	words := strings.Split(strings.ReplaceAll(id, "_", "-"), "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

type (
	ConfigQueryDef struct {
		ID    string `yaml:"id" json:"id"`
		Query string `yaml:"query" json:"query"`
	}

	ConfigMetricBlock struct {
		ID            string               `yaml:"id" json:"id"`
		QueryRef      string               `yaml:"query_ref,omitempty" json:"query_ref"`
		Query         string               `yaml:"query,omitempty" json:"query"`
		Mode          string               `yaml:"mode" json:"mode"` // "columns" | "kv"
		KVMode        *ConfigKVMode        `yaml:"kv_mode,omitempty" json:"kv_mode"`
		LabelsFromRow []ConfigLabelFromRow `yaml:"labels_from_row,omitempty" json:"labels_from_row"`
		Charts        []ConfigChartConfig  `yaml:"charts,omitempty" json:"charts"`
	}
	ConfigKVMode struct {
		NameCol  string `yaml:"name_col,omitempty" json:"name_col"`
		ValueCol string `yaml:"value_col,omitempty" json:"value_col"`
	}
	ConfigLabelFromRow struct {
		Source string `yaml:"source" json:"source"`
		Name   string `yaml:"name" json:"name"`
		//ValueMap map[string]string `yaml:"value_map" json:"value_map"`
	}
)

type (
	ConfigChartConfig struct {
		Title     string            `yaml:"title" json:"title"`
		Context   string            `yaml:"context" json:"context"`
		Family    string            `yaml:"family" json:"family"`
		Type      string            `yaml:"type" json:"type"`
		Units     string            `yaml:"units" json:"units"`
		Algorithm string            `yaml:"algorithm" json:"algorithm"`
		Dims      []ConfigDimConfig `yaml:"dims" json:"dims"`
	}
	ConfigDimConfig struct {
		Name       string            `yaml:"name" json:"name"`
		Source     string            `yaml:"source" json:"source"`
		StatusWhen *ConfigStatusWhen `yaml:"status_when,omitempty" json:"status_when,omitempty"`
	}
	ConfigStatusWhen struct {
		Equals string   `yaml:"equals,omitempty" json:"equals,omitempty"`
		In     []string `yaml:"in,omitempty" json:"in,omitempty"`
		Match  string   `yaml:"match,omitempty" json:"match,omitempty"`

		re *regexp.Regexp
	}
)

func (c *Collector) validateConfig() error {
	var errs []error

	if c.Driver == "" {
		errs = append(errs, errors.New("driver required"))
	} else if !supportedDrivers[c.Driver] {
		errs = append(errs, fmt.Errorf("unsupported driver %q", c.Driver))
	}
	if c.DSN == "" {
		errs = append(errs, errors.New("dsn required"))
	}

	if c.FunctionOnly {
		if len(c.Metrics) > 0 {
			errs = append(errs, errors.New("function_only is set but metrics are defined"))
		}
		if len(c.Functions) == 0 {
			errs = append(errs, errors.New("function_only is set but no functions defined"))
		}
	} else {
		if len(c.Metrics) == 0 {
			errs = append(errs, errors.New("metrics required (or set function_only: true)"))
		}
	}

	queryIdx := map[string]bool{}
	for i := range c.Queries {
		errs = append(errs, c.Queries[i].validate(i, queryIdx)...)
	}

	for i := range c.Metrics {
		errs = append(errs, c.Metrics[i].validate(i, queryIdx)...)
	}

	funcIdx := map[string]bool{}
	for i := range c.Functions {
		errs = append(errs, c.Functions[i].validate(i, funcIdx)...)
	}

	return errors.Join(errs...)
}

func (f *ConfigFunction) validate(idx int, seen map[string]bool) []error {
	var errs []error
	fidx := idx + 1

	if f.ID == "" {
		errs = append(errs, fmt.Errorf("functions[%d] missing id", fidx))
	}
	if f.Query == "" {
		errs = append(errs, fmt.Errorf("functions[%d] missing query", fidx))
	}

	if f.ID != "" {
		if _, dup := seen[f.ID]; dup {
			errs = append(errs, fmt.Errorf("functions[%d] duplicate id %q", fidx, f.ID))
		}
		seen[f.ID] = true
	}

	if f.Limit < 0 {
		errs = append(errs, fmt.Errorf("functions[%d] limit cannot be negative", fidx))
	}
	if f.Limit > maxFunctionLimit {
		errs = append(errs, fmt.Errorf("functions[%d] limit exceeds maximum (%d)", fidx, maxFunctionLimit))
	}
	if f.Timeout.Duration() < 0 {
		errs = append(errs, fmt.Errorf("functions[%d] timeout cannot be negative", fidx))
	}

	validTypes := map[string]bool{
		"string": true, "integer": true, "float": true,
		"boolean": true, "duration": true, "timestamp": true,
	}
	for colName, col := range f.Columns {
		if col.Type != "" && !validTypes[col.Type] {
			errs = append(errs, fmt.Errorf("functions[%d] column %q invalid type %q", fidx, colName, col.Type))
		}
	}

	return errs
}

// ---- Per-struct validation helpers ----

func (q *ConfigQueryDef) validate(idx int, seen map[string]bool) []error {
	var errs []error
	qidx := idx + 1

	if q.ID == "" {
		errs = append(errs, fmt.Errorf("queries[%d] missing id", qidx))
	}
	if q.Query == "" {
		errs = append(errs, fmt.Errorf("queries[%d] missing query", qidx))
	}

	if q.ID != "" {
		if _, dup := seen[q.ID]; dup {
			errs = append(errs, fmt.Errorf("queries[%d] duplicate id %q", qidx, q.ID))
		}
		seen[q.ID] = true
	}

	return errs
}

func (m *ConfigMetricBlock) validate(idx int, queryIdx map[string]bool) []error {
	var errs []error
	midx := idx + 1

	if m.ID == "" {
		errs = append(errs, fmt.Errorf("metrics[%d] missing id", midx))
	}

	hasRef := strings.TrimSpace(m.QueryRef) != ""
	hasInline := strings.TrimSpace(m.Query) != ""
	switch {
	case hasRef && hasInline:
		errs = append(errs, fmt.Errorf("metrics[%d] must set exactly one of query_ref or query, not both", midx))
	case !hasRef && !hasInline:
		errs = append(errs, fmt.Errorf("metrics[%d] must set exactly one of query_ref or query", midx))
	case hasRef:
		if _, ok := queryIdx[m.QueryRef]; !ok {
			errs = append(errs, fmt.Errorf("metrics[%d] query_ref %q not found in queries", midx, m.QueryRef))
		}
	}

	mode := strings.ToLower(strings.TrimSpace(m.Mode))
	switch mode {
	case "kv":
		if m.KVMode == nil {
			errs = append(errs, fmt.Errorf("metrics[%d] kv mode requires kv_mode to be defined", midx))
		} else {
			if strings.TrimSpace(m.KVMode.NameCol) == "" {
				errs = append(errs, fmt.Errorf("metrics[%d] kv_mode.name_col is required in kv mode", midx))
			}
			if strings.TrimSpace(m.KVMode.ValueCol) == "" {
				errs = append(errs, fmt.Errorf("metrics[%d] kv_mode.value_col is required in kv mode", midx))
			}
			if strings.EqualFold(m.KVMode.NameCol, m.KVMode.ValueCol) {
				errs = append(errs, fmt.Errorf("metrics[%d] kv_mode.name_col must differ from kv_mode.value_col", midx))
			}
		}
	case "columns", "":
	default:
		errs = append(errs, fmt.Errorf("metrics[%d] invalid mode %q (expected: columns|kv)", midx, m.Mode))
	}

	labelCols := map[string]bool{}
	for j := range m.LabelsFromRow {
		errs = append(errs, m.LabelsFromRow[j].validate(midx, j, labelCols)...)
	}

	if len(m.Charts) == 0 {
		errs = append(errs, fmt.Errorf("metrics[%d].%s missing charts", midx, m.ID))
	}

	for k := range m.Charts {
		errs = append(errs, m.Charts[k].validate(idx, m.ID, k, mode, labelCols, m.KVMode)...)
	}

	return errs
}

func (lf *ConfigLabelFromRow) validate(metricIdx, lfIdx int, labelCols map[string]bool) []error {
	var errs []error
	midx := metricIdx + 1
	lidx := lfIdx + 1

	if lf.Source == "" {
		errs = append(errs, fmt.Errorf("metrics[%d].labels_from_row[%d] missing source", midx, lidx))
	}
	if lf.Name == "" {
		errs = append(errs, fmt.Errorf("metrics[%d].labels_from_row[%d] missing name", midx, lidx))
	}
	if lf.Source != "" {
		labelCols[strings.ToLower(lf.Source)] = true
	}

	return errs
}

func (ch *ConfigChartConfig) validate(metricIdx int, metricID string, chartIdx int, mode string, labelCols map[string]bool, kv *ConfigKVMode) []error {
	var errs []error
	midx := metricIdx + 1
	cidx := chartIdx + 1

	if strings.TrimSpace(ch.Context) == "" {
		errs = append(errs, fmt.Errorf("metrics[%d].charts[%d] missing context", midx, cidx))
	}
	if strings.TrimSpace(ch.Title) == "" {
		errs = append(errs, fmt.Errorf("metrics[%d].charts[%d] missing title", midx, cidx))
	}
	if strings.TrimSpace(ch.Family) == "" {
		errs = append(errs, fmt.Errorf("metrics[%d].charts[%d] missing family", midx, cidx))
	}
	if strings.TrimSpace(ch.Units) == "" {
		errs = append(errs, fmt.Errorf("metrics[%d].charts[%d] missing units", midx, cidx))
	}

	seenDims := map[string]bool{}
	for d := range ch.Dims {
		errs = append(errs, ch.Dims[d].validate(metricIdx, metricID, chartIdx, d, mode, labelCols, kv, seenDims)...)
	}

	return errs
}

func (dm *ConfigDimConfig) validate(
	metricIdx int,
	metricID string,
	chartIdx int,
	dimIdx int,
	mode string,
	labelCols map[string]bool,
	kv *ConfigKVMode,
	seenDims map[string]bool,
) []error {
	var errs []error
	midx := metricIdx + 1
	cidx := chartIdx + 1
	didx := dimIdx + 1

	if strings.TrimSpace(dm.Name) == "" {
		errs = append(errs, fmt.Errorf("metrics[%d].charts[%d].dims[%d] missing name", midx, cidx, didx))
	}
	if strings.TrimSpace(dm.Source) == "" {
		errs = append(errs, fmt.Errorf("metrics[%d].charts[%d].dims[%d] missing source", midx, cidx, didx))
	}

	// unique dim names (case-insensitive)
	if dm.Name != "" {
		key := strings.ToLower(dm.Name)
		if _, ok := seenDims[key]; ok {
			errs = append(errs, fmt.Errorf("metrics[%d].charts[%d] duplicate dim name %q", midx, cidx, dm.Name))
		} else {
			seenDims[key] = true
		}
	}

	if dm.StatusWhen != nil {
		errs = append(errs, dm.StatusWhen.validate(metricIdx, metricID, chartIdx, dimIdx)...)
	}

	switch mode {
	case "columns":
		if _, clash := labelCols[strings.ToLower(dm.Source)]; clash {
			errs = append(errs, fmt.Errorf("metrics[%d].charts[%d].dims[%d] source %q conflicts with labels_from_row source", midx, cidx, didx, dm.Source))
		}
	case "kv":
		// In kv mode, dim.source must be a KEY name (not a column).
		if kv != nil && strings.EqualFold(dm.Source, kv.ValueCol) {
			errs = append(errs, fmt.Errorf("metrics[%d].charts[%d].dims[%d] source %q equals kv_mode.value_col; dim.source must be a KEY name", midx, cidx, didx, dm.Source))
		}
	}

	return errs
}

func (sw *ConfigStatusWhen) validate(metricIdx int, metricID string, chartIdx, dimIdx int) []error {
	var errs []error
	midx := metricIdx + 1
	cidx := chartIdx + 1
	didx := dimIdx + 1

	count := 0
	if sw.Equals != "" {
		count++
	}
	if len(sw.In) > 0 {
		count++
	}
	if strings.TrimSpace(sw.Match) != "" {
		re, err := regexp.Compile(sw.Match)
		if err != nil {
			errs = append(errs, fmt.Errorf("invalid regex in status_when.match for metric %q: %w", metricID, err))
		} else {
			// store compiled regex for later use
			sw.re = re
		}
		count++
	}

	if count == 0 {
		errs = append(errs, fmt.Errorf("metrics[%d].charts[%d].dims[%d].status_when must have exactly one of equals|in|match", midx, cidx, didx))
	}
	if count > 1 {
		errs = append(errs, fmt.Errorf("metrics[%d].charts[%d].dims[%d].status_when must not set multiple selectors", midx, cidx, didx))
	}

	return errs
}
