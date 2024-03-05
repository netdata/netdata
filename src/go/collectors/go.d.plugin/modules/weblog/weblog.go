// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	_ "embed"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/logs"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("web_log", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
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
		logs.ParserConfig   `yaml:",inline" json:""`
		UpdateEvery         int                  `yaml:"update_every" json:"update_every"`
		Path                string               `yaml:"path" json:"path"`
		ExcludePath         string               `yaml:"exclude_path" json:"exclude_path"`
		URLPatterns         []userPattern        `yaml:"url_patterns" json:"url_patterns"`
		CustomFields        []customField        `yaml:"custom_fields" json:"custom_fields"`
		CustomTimeFields    []customTimeField    `yaml:"custom_time_fields" json:"custom_time_fields"`
		CustomNumericFields []customNumericField `yaml:"custom_numeric_fields" json:"custom_numeric_fields"`
		Histogram           []float64            `yaml:"histogram" json:"histogram"`
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
		Multiplier int    `yaml:"multiplier" json:"multiplier"`
		Divisor    int    `yaml:"divisor" json:"divisor"`
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
		w.Errorf("init failed: %v", err)
		return err
	}

	if err := w.createCustomFields(); err != nil {
		w.Errorf("init failed: %v", err)
		return err
	}

	if err := w.createCustomTimeFields(); err != nil {
		w.Errorf("init failed: %v", err)
		return err
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
		w.Warning("check failed: ", err)
		return err
	}

	if err := w.createParser(); err != nil {
		w.Warning("check failed: ", err)
		return err
	}

	if err := w.createCharts(w.line); err != nil {
		w.Warning("check failed: ", err)
		return err
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
