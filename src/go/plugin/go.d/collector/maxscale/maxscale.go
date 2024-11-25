// SPDX-License-Identifier: GPL-3.0-or-later

package maxscale

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("maxscale", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *MaxScale {
	return &MaxScale{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL:      "http://127.0.0.1:8989",
					Username: "admin",
					Password: "mariadb",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 1),
				},
			},
		},
		charts:      charts.Copy(),
		seenServers: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type MaxScale struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client

	seenServers map[string]bool
}

func (m *MaxScale) Configuration() any {
	return m.Config
}

func (m *MaxScale) Init() error {
	if m.URL == "" {
		return errors.New("URL required but not set")
	}

	httpClient, err := web.NewHTTPClient(m.ClientConfig)
	if err != nil {
		return fmt.Errorf("failed initializing http client: %w", err)
	}
	m.httpClient = httpClient

	m.Debugf("using URL %s", m.URL)
	m.Debugf("using timeout: %s", m.Timeout)

	return nil
}

func (m *MaxScale) Check() error {
	mx, err := m.collect()
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (m *MaxScale) Charts() *module.Charts {
	return m.charts
}

func (m *MaxScale) Collect() map[string]int64 {
	mx, err := m.collect()
	if err != nil {
		m.Error(err)
		return nil
	}

	return mx
}

func (m *MaxScale) Cleanup() {
	if m.httpClient != nil {
		m.httpClient.CloseIdleConnections()
	}
}
