// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import (
	_ "embed"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/logs"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("squidlog", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *SquidLog {
	cfg := logs.ParserConfig{
		LogType: logs.TypeCSV,
		CSV: logs.CSVConfig{
			FieldsPerRecord:  -1,
			Delimiter:        " ",
			TrimLeadingSpace: true,
			Format:           "- $resp_time $client_address $result_code $resp_size $req_method - - $hierarchy $mime_type",
			CheckField:       checkCSVFormatField,
		},
	}
	return &SquidLog{
		Config: Config{
			Path:        "/var/log/squid/access.log",
			ExcludePath: "*.gz",
			Parser:      cfg,
		},
	}
}

type (
	Config struct {
		Parser      logs.ParserConfig `yaml:",inline"`
		Path        string            `yaml:"path"`
		ExcludePath string            `yaml:"exclude_path"`
	}

	SquidLog struct {
		module.Base
		Config `yaml:",inline"`

		file   *logs.Reader
		parser logs.Parser
		line   *logLine

		mx     *metricsData
		charts *module.Charts
	}
)

func (s *SquidLog) Init() bool {
	s.line = newEmptyLogLine()
	s.mx = newMetricsData()
	return true
}

func (s *SquidLog) Check() bool {
	// Note: these inits are here to make auto-detection retry working
	if err := s.createLogReader(); err != nil {
		s.Warning("check failed: ", err)
		return false
	}

	if err := s.createParser(); err != nil {
		s.Warning("check failed: ", err)
		return false
	}

	if err := s.createCharts(s.line); err != nil {
		s.Warning("check failed: ", err)
		return false
	}
	return true
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
