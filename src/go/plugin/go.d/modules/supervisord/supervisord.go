// SPDX-License-Identifier: GPL-3.0-or-later

package supervisord

import (
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("supervisord", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Supervisord {
	return &Supervisord{
		Config: Config{
			URL: "http://127.0.0.1:9001/RPC2",
			ClientConfig: web.ClientConfig{
				Timeout: confopt.Duration(time.Second),
			},
		},

		charts: summaryCharts.Copy(),
		cache:  make(map[string]map[string]bool),
	}
}

type Config struct {
	UpdateEvery      int    `yaml:"update_every,omitempty" json:"update_every"`
	URL              string `yaml:"url" json:"url"`
	web.ClientConfig `yaml:",inline" json:""`
}

type (
	Supervisord struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		client supervisorClient

		cache map[string]map[string]bool // map[group][procName]collected
	}
	supervisorClient interface {
		getAllProcessInfo() ([]processStatus, error)
		closeIdleConnections()
	}
)

func (s *Supervisord) Configuration() any {
	return s.Config
}

func (s *Supervisord) Init() error {
	err := s.verifyConfig()
	if err != nil {
		return fmt.Errorf("verify config: %v", err)
	}

	client, err := s.initSupervisorClient()
	if err != nil {
		return fmt.Errorf("init supervisord client: %v", err)
	}
	s.client = client

	return nil
}

func (s *Supervisord) Check() error {
	mx, err := s.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
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
