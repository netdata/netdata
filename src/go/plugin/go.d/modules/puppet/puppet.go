// SPDX-License-Identifier: GPL-3.0-or-later

package puppet

import (
	_ "embed"
	"errors"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("puppet", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Puppet {
	return &Puppet{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "https://127.0.0.1:8140",
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

type Puppet struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
}

func (p *Puppet) Configuration() any {
	return p.Config
}

func (p *Puppet) Init() error {
	if p.URL == "" {
		p.Error("URL not set")
		return errors.New("url not set")
	}

	client, err := web.NewHTTPClient(p.ClientConfig)
	if err != nil {
		p.Error(err)
		return err
	}
	p.httpClient = client

	p.Debugf("using URL %s", p.URL)
	p.Debugf("using timeout: %s", p.Timeout)

	return nil
}

func (p *Puppet) Check() error {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (p *Puppet) Charts() *module.Charts {
	return p.charts
}

func (p *Puppet) Collect() map[string]int64 {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (p *Puppet) Cleanup() {
	if p.httpClient != nil {
		p.httpClient.CloseIdleConnections()
	}
}
