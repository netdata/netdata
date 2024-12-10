// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("squidlog", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Path:        "/var/log/squid/access.log",
			ExcludePath: "*.gz",
			ParserConfig: logs.ParserConfig{
				LogType: logs.TypeCSV,
				CSV: logs.CSVConfig{
					FieldsPerRecord:  -1,
					Delimiter:        " ",
					TrimLeadingSpace: true,
					Format:           "- $resp_time $client_address $result_code $resp_size $req_method - - $hierarchy $mime_type",
					CheckField:       checkCSVFormatField,
				},
			},
		},
	}
}

type Config struct {
	UpdateEvery       int    `yaml:"update_every,omitempty" json:"update_every"`
	Path              string `yaml:"path" json:"path"`
	ExcludePath       string `yaml:"exclude_path,omitempty" json:"exclude_path"`
	logs.ParserConfig `yaml:",inline" json:""`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	file   *logs.Reader
	parser logs.Parser
	line   *logLine

	mx *metricsData
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	c.line = newEmptyLogLine()
	c.mx = newMetricsData()
	return nil
}

func (c *Collector) Check(context.Context) error {
	// Note: these inits are here to make auto-detection retry working
	if err := c.createLogReader(); err != nil {
		return fmt.Errorf("failed to create log reader: %v", err)
	}

	if err := c.createParser(); err != nil {
		return fmt.Errorf("failed to create log parser: %v", err)
	}

	if err := c.createCharts(c.line); err != nil {
		return fmt.Errorf("failed to create log charts: %v", err)
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
