// SPDX-License-Identifier: GPL-3.0-or-later

package scaleio

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/modules/scaleio/client"
	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/netdata/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("scaleio", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

// New creates ScaleIO with default values.
func New() *ScaleIO {
	config := Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: "https://127.0.0.1",
			},
			Client: web.Client{
				Timeout: web.Duration{Duration: time.Second},
			},
		},
	}
	return &ScaleIO{
		Config:  config,
		charts:  systemCharts.Copy(),
		charted: make(map[string]bool),
	}
}

type (
	// Config is the ScaleIO module configuration.
	Config struct {
		web.HTTP `yaml:",inline"`
	}
	// ScaleIO ScaleIO module.
	ScaleIO struct {
		module.Base
		Config `yaml:",inline"`
		client *client.Client
		charts *module.Charts

		discovered instances
		charted    map[string]bool

		lastDiscoveryOK bool
		runs            int
	}
	instances struct {
		sdc  map[string]client.Sdc
		pool map[string]client.StoragePool
	}
)

// Init makes initialization.
func (s *ScaleIO) Init() bool {
	if s.Username == "" || s.Password == "" {
		s.Error("username and password aren't set")
		return false
	}

	c, err := client.New(s.Client, s.Request)
	if err != nil {
		s.Errorf("error on creating ScaleIO client: %v", err)
		return false
	}
	s.client = c

	s.Debugf("using URL %s", s.URL)
	s.Debugf("using timeout: %s", s.Timeout.Duration)
	return true
}

// Check makes check.
func (s *ScaleIO) Check() bool {
	if err := s.client.Login(); err != nil {
		s.Error(err)
		return false
	}
	return len(s.Collect()) > 0
}

// Charts returns Charts.
func (s *ScaleIO) Charts() *module.Charts {
	return s.charts
}

// Collect collects metrics.
func (s *ScaleIO) Collect() map[string]int64 {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
		return nil
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

// Cleanup makes cleanup.
func (s *ScaleIO) Cleanup() {
	if s.client == nil {
		return
	}
	_ = s.client.Logout()
}
