// SPDX-License-Identifier: GPL-3.0-or-later

package scaleio

import (
	_ "embed"
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/modules/scaleio/client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("scaleio", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *ScaleIO {
	return &ScaleIO{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "https://127.0.0.1",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
		},
		charts:  systemCharts.Copy(),
		charted: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type (
	ScaleIO struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		client *client.Client

		discovered      instances
		charted         map[string]bool
		lastDiscoveryOK bool
		runs            int
	}
	instances struct {
		sdc  map[string]client.Sdc
		pool map[string]client.StoragePool
	}
)

func (s *ScaleIO) Configuration() any {
	return s.Config
}

func (s *ScaleIO) Init() error {
	if s.Username == "" || s.Password == "" {
		s.Error("username and password aren't set")
		return errors.New("username and password aren't set")
	}

	c, err := client.New(s.ClientConfig, s.RequestConfig)
	if err != nil {
		s.Errorf("error on creating ScaleIO client: %v", err)
		return err
	}
	s.client = c

	s.Debugf("using URL %s", s.URL)
	s.Debugf("using timeout: %s", s.Timeout)

	return nil
}

func (s *ScaleIO) Check() error {
	if err := s.client.Login(); err != nil {
		s.Error(err)
		return err
	}
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (s *ScaleIO) Charts() *module.Charts {
	return s.charts
}

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

func (s *ScaleIO) Cleanup() {
	if s.client == nil {
		return
	}
	_ = s.client.Logout()
}
