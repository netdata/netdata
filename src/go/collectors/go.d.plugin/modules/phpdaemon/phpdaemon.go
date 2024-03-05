// SPDX-License-Identifier: GPL-3.0-or-later

package phpdaemon

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("phpdaemon", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *PHPDaemon {
	return &PHPDaemon{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:8509/FullStatus",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
		},
		charts: charts.Copy(),
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int `yaml:"update_every" json:"update_every"`
}

type PHPDaemon struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *Charts

	client *client
}

func (p *PHPDaemon) Configuration() any {
	return p.Config
}

func (p *PHPDaemon) Init() error {
	if err := p.validateConfig(); err != nil {
		p.Error(err)
		return err
	}

	c, err := p.initClient()
	if err != nil {
		p.Error(err)
		return err
	}
	p.client = c

	p.Debugf("using URL %s", p.URL)
	p.Debugf("using timeout: %s", p.Timeout)

	return nil
}

func (p *PHPDaemon) Check() error {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	if _, ok := mx["uptime"]; ok {
		_ = p.charts.Add(uptimeChart.Copy())
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
	if p.client != nil && p.client.httpClient != nil {
		p.client.httpClient.CloseIdleConnections()
	}
}
