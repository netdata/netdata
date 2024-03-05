// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/logger"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

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
			Interval:    web.Duration(time.Millisecond * 100),
		},

		charts:    &module.Charts{},
		hosts:     make(map[string]bool),
		newProber: newPingProber,
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every" json:"update_every"`
	Hosts       []string     `yaml:"hosts" json:"hosts"`
	Network     string       `yaml:"network" json:"network"`
	Privileged  bool         `yaml:"privileged" json:"privileged"`
	SendPackets int          `yaml:"packets" json:"packets"`
	Interval    web.Duration `yaml:"interval" json:"interval"`
	Interface   string       `yaml:"interface" json:"interface"`
}

type (
	Ping struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		prober    prober
		newProber func(pingProberConfig, *logger.Logger) prober

		hosts map[string]bool
	}
	prober interface {
		ping(host string) (*probing.Statistics, error)
	}
)

func (p *Ping) Configuration() any {
	return p.Config
}

func (p *Ping) Init() error {
	err := p.validateConfig()
	if err != nil {
		p.Errorf("config validation: %v", err)
		return err
	}

	pr, err := p.initProber()
	if err != nil {
		p.Errorf("init prober: %v", err)
		return err
	}
	p.prober = pr

	return nil
}

func (p *Ping) Check() error {
	mx, err := p.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
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
