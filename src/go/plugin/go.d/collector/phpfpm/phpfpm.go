// SPDX-License-Identifier: GPL-3.0-or-later

package phpfpm

import (
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("phpfpm", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Phpfpm {
	return &Phpfpm{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1/status?full&json",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
			FcgiPath: "/status",
		},
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
	Socket         string `yaml:"socket,omitempty" json:"socket"`
	Address        string `yaml:"address,omitempty" json:"address"`
	FcgiPath       string `yaml:"fcgi_path,omitempty" json:"fcgi_path"`
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
		return fmt.Errorf("init client: %v", err)
	}
	p.client = c

	return nil
}

func (p *Phpfpm) Check() error {
	mx, err := p.collect()
	if err != nil {
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
