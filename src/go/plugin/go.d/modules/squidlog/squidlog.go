// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import (
	_ "embed"

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

func New() *SquidLog {
	return &SquidLog{
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

type SquidLog struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	file   *logs.Reader
	parser logs.Parser
	line   *logLine

	mx *metricsData
}

func (s *SquidLog) Configuration() any {
	return s.Config
}

func (s *SquidLog) Init() error {
	s.line = newEmptyLogLine()
	s.mx = newMetricsData()
	return nil
}

func (s *SquidLog) Check() error {
	// Note: these inits are here to make auto-detection retry working
	if err := s.createLogReader(); err != nil {
		s.Warning("check failed: ", err)
		return err
	}

	if err := s.createParser(); err != nil {
		s.Warning("check failed: ", err)
		return err
	}

	if err := s.createCharts(s.line); err != nil {
		s.Warning("check failed: ", err)
		return err
	}

	return nil
}

func (s *SquidLog) Charts() *module.Charts {
	return s.charts
}

func (s *SquidLog) Collect() map[string]int64 {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (s *SquidLog) Cleanup() {
	if s.file != nil {
		_ = s.file.Close()
	}
}
