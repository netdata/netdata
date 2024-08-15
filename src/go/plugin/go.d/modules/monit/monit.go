// SPDX-License-Identifier: GPL-3.0-or-later

package monit

import (
	_ "embed"
	"errors"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("monit", module.Creator{
		Create:          func() module.Module { return New() },
		JobConfigSchema: configSchema,
		Config:          func() any { return &Config{} },
	})
}

func New() *Monit {
	return &Monit{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL:      "http://127.0.0.1:2812",
					Username: "admin",
					Password: "monit",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
		},
		charts:       baseCharts.Copy(),
		seenServices: make(map[string]statusServiceCheck),
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTP    `yaml:",inline" json:""`
}

type Monit struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client

	seenServices map[string]statusServiceCheck
}

func (m *Monit) Configuration() any {
	return m.Config
}

func (m *Monit) Init() error {
	if m.URL == "" {
		m.Error("config: monit url is required but not set")
		return errors.New("config: missing URL")
	}

	httpClient, err := web.NewHTTPClient(m.Client)
	if err != nil {
		m.Errorf("init HTTP client: %v", err)
		return err
	}
	m.httpClient = httpClient

	m.Debugf("using URL %s", m.URL)
	m.Debugf("using timeout: %s", m.Timeout)

	return nil
}

func (m *Monit) Check() error {
	mx, err := m.collect()
	if err != nil {
		m.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (m *Monit) Charts() *module.Charts {
	return m.charts
}

func (m *Monit) Collect() map[string]int64 {
	mx, err := m.collect()
	if err != nil {
		m.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (m *Monit) Cleanup() {
	if m.httpClient != nil {
		m.httpClient.CloseIdleConnections()
	}
}
