// SPDX-License-Identifier: GPL-3.0-or-later

package phpdaemon

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("phpdaemon", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *PHPDaemon {
	return &PHPDaemon{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:8509/FullStatus",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
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

type PHPDaemon struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *Charts
	once   sync.Once

	httpClient *http.Client
}

func (p *PHPDaemon) Configuration() any {
	return p.Config
}

func (p *PHPDaemon) Init() error {
	if p.URL == "" {
		return errors.New("phpDaemon URL is required but not set")
	}

	httpClient, err := web.NewHTTPClient(p.ClientConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize http client: %w", err)
	}
	p.httpClient = httpClient

	p.Debugf("using URL %s", p.URL)
	p.Debugf("using timeout: %s", p.Timeout)

	return nil
}

func (p *PHPDaemon) Check() error {
	mx, err := p.collect()
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (p *PHPDaemon) Charts() *Charts {
	return p.charts
}

func (p *PHPDaemon) Collect() map[string]int64 {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
		return nil
	}

	return mx
}

func (p *PHPDaemon) Cleanup() {
	if p.httpClient != nil {
		p.httpClient.CloseIdleConnections()
	}
}
