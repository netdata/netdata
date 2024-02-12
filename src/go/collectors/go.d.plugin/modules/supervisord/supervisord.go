// SPDX-License-Identifier: GPL-3.0-or-later

package supervisord

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("supervisord", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Supervisord {
	return &Supervisord{
		Config: Config{
			URL: "http://127.0.0.1:9001/RPC2",
			Client: web.Client{
				Timeout: web.Duration{Duration: time.Second},
			},
		},

		charts: summaryCharts.Copy(),
		cache:  make(map[string]map[string]bool),
	}
}

type Config struct {
	URL        string `yaml:"url"`
	web.Client `yaml:",inline"`
}

type (
	Supervisord struct {
		module.Base
		Config `yaml:",inline"`

		client supervisorClient
		charts *module.Charts

		cache map[string]map[string]bool // map[group][procName]collected
	}
	supervisorClient interface {
		getAllProcessInfo() ([]processStatus, error)
		closeIdleConnections()
	}
)

func (s *Supervisord) Init() bool {
	err := s.verifyConfig()
	if err != nil {
		s.Errorf("verify config: %v", err)
		return false
	}

	client, err := s.initSupervisorClient()
	if err != nil {
		s.Errorf("init supervisord client: %v", err)
		return false
	}
	s.client = client

	return true
}

func (s *Supervisord) Check() bool {
	return len(s.Collect()) > 0
}

func (s *Supervisord) Charts() *module.Charts {
	return s.charts
}

func (s *Supervisord) Collect() map[string]int64 {
	ms, err := s.collect()
	if err != nil {
		s.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (s *Supervisord) Cleanup() {
	if s.client != nil {
		s.client.closeIdleConnections()
	}
}
