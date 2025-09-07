// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
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

func New() *Collector {
	return &Collector{
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
		GroupRespCodes      confopt.FlexBool     `yaml:"group_response_codes" json:"group_response_codes"`
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

type Collector struct {
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

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.createURLPatterns(); err != nil {
		return fmt.Errorf("init failed: %v", err)
	}

	if err := c.createCustomFields(); err != nil {
		return fmt.Errorf("init failed: %v", err)
	}

	if err := c.createCustomTimeFields(); err != nil {
		return fmt.Errorf("init failed: %v", err)
	}

	if err := c.createCustomNumericFields(); err != nil {
		c.Errorf("init failed: %v", err)
	}

	c.createLogLine()
	c.mx = newMetricsData(c.Config)

	return nil
}

func (c *Collector) Check(context.Context) error {
	// Note: these inits are here to make auto-detection retry working
	if err := c.createLogReader(); err != nil {
		return fmt.Errorf("failed to create log reader: %v", err)
	}

	if err := c.createParser(); err != nil {
		return fmt.Errorf("failed to create parser: %v", err)
	}

	if err := c.createCharts(c.line); err != nil {
		return fmt.Errorf("failed to create charts: %v", err)
	}

	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (c *Collector) Cleanup(context.Context) {
	if c.file != nil {
		_ = c.file.Close()
	}
}
