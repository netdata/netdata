// SPDX-License-Identifier: GPL-3.0-or-later

package scaleio

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/modules/scaleio/client"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("scaleio", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *ScaleIO {
	return &ScaleIO{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "https://127.0.0.1",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
		},
		charts:  systemCharts.Copy(),
		charted: make(map[string]bool),
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int `yaml:"update_every" json:"update_every"`
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

	c, err := client.New(s.Client, s.Request)
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
