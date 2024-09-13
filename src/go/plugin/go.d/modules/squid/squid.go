// SPDX-License-Identifier: GPL-3.0-or-later

package squid

import (
	_ "embed"
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("squid", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Squid {
	return &Squid{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:3128",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 1),
				},
			},
		},
		charts: charts.Copy(),
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type Squid struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
}

func (s *Squid) Configuration() any {
	return s.Config
}

func (s *Squid) Init() error {
	if s.URL == "" {
		s.Error("URL not set")
		return errors.New("url not set")
	}

	client, err := web.NewHTTPClient(s.ClientConfig)
	if err != nil {
		s.Error(err)
		return err
	}
	s.httpClient = client

	s.Debugf("using URL %s", s.URL)
	s.Debugf("using timeout: %s", s.Timeout)

	return nil
}

func (s *Squid) Check() error {
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

func (s *Squid) Charts() *module.Charts {
	return s.charts
}

func (s *Squid) Collect() map[string]int64 {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (s *Squid) Cleanup() {
	if s.httpClient != nil {
		s.httpClient.CloseIdleConnections()
	}
}
