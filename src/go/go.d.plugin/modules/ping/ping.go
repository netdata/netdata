// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/logger"
	"github.com/netdata/go.d.plugin/pkg/web"

	probing "github.com/prometheus-community/pro-bing"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("ping", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *Ping {
	return &Ping{
		Config: Config{
			Network:     "ip",
			Privileged:  true,
			SendPackets: 5,
			Interval:    web.Duration{Duration: time.Millisecond * 100},
		},

		charts:    &module.Charts{},
		hosts:     make(map[string]bool),
		newProber: newPingProber,
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every"`
	Hosts       []string     `yaml:"hosts"`
	Network     string       `yaml:"network"`
	Privileged  bool         `yaml:"privileged"`
	SendPackets int          `yaml:"packets"`
	Interval    web.Duration `yaml:"interval"`
	Interface   string       `yaml:"interface"`
}

type (
	Ping struct {
		module.Base
		Config `yaml:",inline"`

		charts *module.Charts

		hosts map[string]bool

		newProber func(pingProberConfig, *logger.Logger) prober
		prober    prober
	}
	prober interface {
		ping(host string) (*probing.Statistics, error)
	}
)

func (p *Ping) Init() bool {
	err := p.validateConfig()
	if err != nil {
		p.Errorf("config validation: %v", err)
		return false
	}

	pr, err := p.initProber()
	if err != nil {
		p.Errorf("init prober: %v", err)
		return false
	}
	p.prober = pr

	return true
}

func (p *Ping) Check() bool {
	return len(p.Collect()) > 0
}

func (p *Ping) Charts() *module.Charts {
	return p.charts
}

func (p *Ping) Collect() map[string]int64 {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (p *Ping) Cleanup() {}
