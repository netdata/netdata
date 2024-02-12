// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	_ "embed"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/logs"
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
			Parser: logs.ParserConfig{
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
		Parser              logs.ParserConfig    `yaml:",inline"`
		Path                string               `yaml:"path"`
		ExcludePath         string               `yaml:"exclude_path"`
		URLPatterns         []userPattern        `yaml:"url_patterns"`
		CustomFields        []customField        `yaml:"custom_fields"`
		CustomTimeFields    []customTimeField    `yaml:"custom_time_fields"`
		CustomNumericFields []customNumericField `yaml:"custom_numeric_fields"`
		Histogram           []float64            `yaml:"histogram"`
		GroupRespCodes      bool                 `yaml:"group_response_codes"`
	}
	userPattern struct {
		Name  string `yaml:"name"`
		Match string `yaml:"match"`
	}
	customField struct {
		Name     string        `yaml:"name"`
		Patterns []userPattern `yaml:"patterns"`
	}
	customTimeField struct {
		Name      string    `yaml:"name"`
		Histogram []float64 `yaml:"histogram"`
	}
	customNumericField struct {
		Name       string `yaml:"name"`
		Units      string `yaml:"units"`
		Multiplier int    `yaml:"multiplier"`
		Divisor    int    `yaml:"divisor"`
	}
)

type WebLog struct {
	module.Base
	Config `yaml:",inline"`

	file        *logs.Reader
	parser      logs.Parser
	line        *logLine
	urlPatterns []*pattern

	customFields        map[string][]*pattern
	customTimeFields    map[string][]float64
	customNumericFields map[string]bool

	charts *module.Charts
	mx     *metricsData
}

func (w *WebLog) Init() bool {
	if err := w.createURLPatterns(); err != nil {
		w.Errorf("init failed: %v", err)
		return false
	}

	if err := w.createCustomFields(); err != nil {
		w.Errorf("init failed: %v", err)
		return false
	}

	if err := w.createCustomTimeFields(); err != nil {
		w.Errorf("init failed: %v", err)
		return false
	}

	if err := w.createCustomNumericFields(); err != nil {
		w.Errorf("init failed: %v", err)
	}

	w.createLogLine()
	w.mx = newMetricsData(w.Config)

	return true
}

func (w *WebLog) Check() bool {
	// Note: these inits are here to make auto-detection retry working
	if err := w.createLogReader(); err != nil {
		w.Warning("check failed: ", err)
		return false
	}

	if err := w.createParser(); err != nil {
		w.Warning("check failed: ", err)
		return false
	}

	if err := w.createCharts(w.line); err != nil {
		w.Warning("check failed: ", err)
		return false
	}
	return true
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
