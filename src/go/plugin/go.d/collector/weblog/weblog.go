// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	_ "embed"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("web_log", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *WebLog {
	return &WebLog{
		Config: Config{
			ExcludePath:    "*.gz",
			GroupRespCodes: true,
			ParserConfig: logs.ParserConfig{
				LogType: typeAuto,
				CSV: logs.CSVConfig{
					FieldsPerRecord:  -1,
					Delimiter:        " ",
					TrimLeadingSpace: false,
					CheckField:       checkCSVFormatField,
				},
				LTSV: logs.LTSVConfig{
					FieldDelimiter: "\t",
					ValueDelimiter: ":",
				},
				RegExp: logs.RegExpConfig{},
				JSON:   logs.JSONConfig{},
			},
		},
	}
}

type (
	Config struct {
		UpdateEvery         int    `yaml:"update_every,omitempty" json:"update_every"`
		Path                string `yaml:"path" json:"path"`
		ExcludePath         string `yaml:"exclude_path,omitempty" json:"exclude_path"`
		logs.ParserConfig   `yaml:",inline" json:""`
		URLPatterns         []userPattern        `yaml:"url_patterns,omitempty" json:"url_patterns"`
		CustomFields        []customField        `yaml:"custom_fields,omitempty" json:"custom_fields"`
		CustomTimeFields    []customTimeField    `yaml:"custom_time_fields,omitempty" json:"custom_time_fields"`
		CustomNumericFields []customNumericField `yaml:"custom_numeric_fields,omitempty" json:"custom_numeric_fields"`
		Histogram           []float64            `yaml:"histogram,omitempty" json:"histogram"`
		GroupRespCodes      bool                 `yaml:"group_response_codes" json:"group_response_codes"`
	}
	userPattern struct {
		Name  string `yaml:"name" json:"name"`
		Match string `yaml:"match" json:"match"`
	}
	customField struct {
		Name     string        `yaml:"name" json:"name"`
		Patterns []userPattern `yaml:"patterns" json:"patterns"`
	}
	customTimeField struct {
		Name      string    `yaml:"name" json:"name"`
		Histogram []float64 `yaml:"histogram" json:"histogram"`
	}
	customNumericField struct {
		Name       string `yaml:"name" json:"name"`
		Units      string `yaml:"units" json:"units"`
		Multiplier int    `yaml:"multiplier,omitempty" json:"multiplier"`
		Divisor    int    `yaml:"divisor,omitempty" json:"divisor"`
	}
)

type WebLog struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	file   *logs.Reader
	parser logs.Parser
	line   *logLine

	urlPatterns         []*pattern
	customFields        map[string][]*pattern
	customTimeFields    map[string][]float64
	customNumericFields map[string]bool

	mx *metricsData
}

func (w *WebLog) Configuration() any {
	return w.Config
}

func (w *WebLog) Init() error {
	if err := w.createURLPatterns(); err != nil {
		return fmt.Errorf("init failed: %v", err)
	}

	if err := w.createCustomFields(); err != nil {
		return fmt.Errorf("init failed: %v", err)
	}

	if err := w.createCustomTimeFields(); err != nil {
		return fmt.Errorf("init failed: %v", err)
	}

	if err := w.createCustomNumericFields(); err != nil {
		w.Errorf("init failed: %v", err)
	}

	w.createLogLine()
	w.mx = newMetricsData(w.Config)

	return nil
}

func (w *WebLog) Check() error {
	// Note: these inits are here to make auto-detection retry working
	if err := w.createLogReader(); err != nil {
		return fmt.Errorf("failed to create log reader: %v", err)
	}

	if err := w.createParser(); err != nil {
		return fmt.Errorf("failed to create parser: %v", err)
	}

	if err := w.createCharts(w.line); err != nil {
		return fmt.Errorf("failed to create charts: %v", err)
	}

	return nil
}

func (w *WebLog) Charts() *module.Charts {
	return w.charts
}

func (w *WebLog) Collect() map[string]int64 {
	mx, err := w.collect()
	if err != nil {
		w.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (w *WebLog) Cleanup() {
	if w.file != nil {
		_ = w.file.Close()
	}
}
