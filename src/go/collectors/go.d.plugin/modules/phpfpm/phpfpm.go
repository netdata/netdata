// SPDX-License-Identifier: GPL-3.0-or-later

package phpfpm

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

////go:embed "config_schema.json"
//var configSchema string

var configSchema string

func init() {
	module.Register("phpfpm", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Phpfpm {
	return &Phpfpm{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1/status?full&json",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
			FcgiPath: "/status",
		},
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int    `yaml:"update_every" json:"update_every"`
	Socket      string `yaml:"socket" json:"socket"`
	Address     string `yaml:"address" json:"address"`
	FcgiPath    string `yaml:"fcgi_path" json:"fcgi_path"`
}

type Phpfpm struct {
	module.Base
	Config `yaml:",inline" json:""`

	client client
}

func (p *Phpfpm) Configuration() any {
	return p.Config
}

func (p *Phpfpm) Init() error {
	c, err := p.initClient()
	if err != nil {
		p.Errorf("init client: %v", err)
		return err
	}
	p.client = c

	return nil
}

func (p *Phpfpm) Check() error {
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

func (p *Phpfpm) Charts() *Charts {
	return charts.Copy()
}

func (p *Phpfpm) Collect() map[string]int64 {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (p *Phpfpm) Cleanup() {}
