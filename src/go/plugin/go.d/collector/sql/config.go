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

	if len(c.Metrics) == 0 {
		errs = append(errs, errors.New("missing metrics"))
	}

	queryIdx := map[string]bool{}
	for i, q := range c.Queries {
		if q.ID == "" {
			errs = append(errs, fmt.Errorf("queries[%d] missing id", i+1))
			continue
		}
		if q.Query == "" {
			errs = append(errs, fmt.Errorf("queries[%d] missing query", i+1))
			continue
		}
		if _, dup := queryIdx[q.ID]; dup {
			errs = append(errs, fmt.Errorf("queries[%d] duplicate id %q", i+1, q.ID))
		}
		queryIdx[q.ID] = true
	}

	for i, m := range c.Metrics {
		idx := i + 1
		if m.ID == "" {
			errs = append(errs, fmt.Errorf("metrics[%d] missing id", idx))
		}

		hasRef := strings.TrimSpace(m.QueryRef) != ""
		hasInline := strings.TrimSpace(m.Query) != ""
		switch {
		case hasRef && hasInline:
			errs = append(errs, fmt.Errorf("metrics[%d] must set exactly one of query_ref or query, not both", idx))
		case !hasRef && !hasInline:
			errs = append(errs, fmt.Errorf("metrics[%d] must set exactly one of query_ref or query", idx))
		case hasRef:
			if _, ok := queryIdx[m.QueryRef]; !ok {
				errs = append(errs, fmt.Errorf("metrics[%d] query_ref %q not found in queries", idx, m.QueryRef))
			}
		}

		mode := strings.ToLower(strings.TrimSpace(m.Mode))
		if mode != "columns" && mode != "kv" {
			errs = append(errs, fmt.Errorf("metrics[%d] invalid mode %q (expected: columns|kv)", idx, m.Mode))
		}

		if mode == "kv" && m.KVMode != nil {
			if m.KVMode.NameCol != "" && m.KVMode.NameCol == m.KVMode.ValueCol {
				errs = append(errs, fmt.Errorf("metrics[%d] kv_mode.name_col must differ from kv_mode.value_col", idx))
			}
		}

		labelCols := map[string]bool{}
		for j, lf := range m.LabelsFromRow {
			if lf.Source == "" {
				errs = append(errs, fmt.Errorf("metrics[%d].labels_from_row[%d] missing source", idx, j+1))
			}
			if lf.Name == "" {
				errs = append(errs, fmt.Errorf("metrics[%d].labels_from_row[%d] missing name", idx, j+1))
			}
			if lf.Source != "" {
				labelCols[strings.ToLower(lf.Source)] = true
			}
		}

		if len(m.Charts) == 0 {
			errs = append(errs, fmt.Errorf("metrics[%d].%s missing charts", idx, m.ID))
		}

		for k, ch := range m.Charts {
			cidx := k + 1

			// context required
			if strings.TrimSpace(ch.Context) == "" {
				errs = append(errs, fmt.Errorf("metrics[%d].charts[%d] missing context", idx, cidx))
			}

			seenDims := map[string]bool{}
			for d, dm := range ch.Dims {
				didx := d + 1
				if strings.TrimSpace(dm.Name) == "" {
					errs = append(errs, fmt.Errorf("metrics[%d].charts[%d].dims[%d] missing name", idx, cidx, didx))
				}
				if strings.TrimSpace(dm.Source) == "" {
					errs = append(errs, fmt.Errorf("metrics[%d].charts[%d].dims[%d] missing source", idx, cidx, didx))
				}

				// unique dim names (case-insensitive)
				key := strings.ToLower(dm.Name)
				if _, ok := seenDims[key]; ok {
					errs = append(errs, fmt.Errorf("metrics[%d].charts[%d] duplicate dim name %q", idx, cidx, dm.Name))
				} else {
					seenDims[key] = true
				}

				if dm.StatusWhen != nil {
					count := 0
					if dm.StatusWhen.Equals != "" {
						count++
					}
					if len(dm.StatusWhen.In) > 0 {
						count++
					}
					if strings.TrimSpace(dm.StatusWhen.Match) != "" {
						re, err := regexp.Compile(dm.StatusWhen.Match)
						if err != nil {
							errs = append(errs, fmt.Errorf("invalid regex in status_when.match for metric %q: %w", m.ID, err))
						} else {
							ch.Dims[d].StatusWhen.re = re
						}
						count++
					}
					if count == 0 {
						errs = append(errs, fmt.Errorf("metrics[%d].charts[%d].dims[%d].status_when must have exactly one of equals|in|match", idx, cidx, didx))
					}
					if count > 1 {
						errs = append(errs, fmt.Errorf("metrics[%d].charts[%d].dims[%d].status_when must not set multiple selectors", idx, cidx, didx))
					}
				}

				switch mode {
				case "columns":
					if _, clash := labelCols[strings.ToLower(dm.Source)]; clash {
						errs = append(errs, fmt.Errorf("metrics[%d].charts[%d].dims[%d] source %q conflicts with labels_from_row source", idx, cidx, didx, dm.Source))
					}
				case "kv":
					// In kv mode, dim.source must be a KEY name (not a column). We can’t fully verify against data,
					// but we can ensure it doesn't equal value_col (which would imply a column misused as key).
					if m.KVMode != nil && strings.EqualFold(dm.Source, m.KVMode.ValueCol) {
						errs = append(errs, fmt.Errorf("metrics[%d].charts[%d].dims[%d] source %q equals kv_mode.value_col; dim.source must be a KEY name", idx, cidx, didx, dm.Source))
					}
				}
			}
		}
	}

	return errors.Join(errs...)
}
