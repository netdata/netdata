// SPDX-License-Identifier: GPL-3.0-or-later

package postfix

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
	module.Register("postfix", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Postfix {
	return &Postfix{
		Config: Config{
			BinaryPath: "/usr/sbin/postqueue",
			Timeout:    web.Duration(time.Second * 2),
		},
		charts: charts.Copy(),
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     web.Duration `yaml:"timeout,omitempty" json:"timeout"`
	BinaryPath  string       `yaml:"binary_path,omitempty" json:"binary_path"`
}

type (
	Postfix struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec postqueueBinary
	}
	postqueueBinary interface {
		list() ([]byte, error)
	}
)

func (p *Postfix) Configuration() any {
	return p.Config
}

func (p *Postfix) Init() error {
	if err := p.validateConfig(); err != nil {
		p.Errorf("config validation: %s", err)
		return err
	}

	pq, err := p.initPostqueueExec()
	if err != nil {
		p.Errorf("postqueue exec initialization: %v", err)
		return err
	}
	p.exec = pq

	return nil
}

func (p *Postfix) Check() error {
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

func (p *Postfix) Charts() *module.Charts {
	return p.charts
}

func (p *Postfix) Collect() map[string]int64 {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (p *Postfix) Cleanup() {}
