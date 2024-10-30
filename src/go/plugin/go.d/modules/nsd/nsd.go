// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package nsd

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("nsd", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Nsd {
	return &Nsd{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
		},
		charts: charts.Copy(),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type Nsd struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	exec nsdControlBinary
}

func (n *Nsd) Configuration() any {
	return n.Config
}

func (n *Nsd) Init() error {
	nsdControl, err := n.initNsdControlExec()
	if err != nil {
		n.Errorf("nsd-control exec initialization: %v", err)
		return err
	}
	n.exec = nsdControl

	return nil
}

func (n *Nsd) Check() error {
	mx, err := n.collect()
	if err != nil {
		n.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (n *Nsd) Charts() *module.Charts {
	return n.charts
}

func (n *Nsd) Collect() map[string]int64 {
	mx, err := n.collect()
	if err != nil {
		n.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (n *Nsd) Cleanup() {}
