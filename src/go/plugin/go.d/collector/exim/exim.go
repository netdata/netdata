// SPDX-License-Identifier: GPL-3.0-or-later

package exim

import (
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("exim", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Exim {
	return &Exim{
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

type Exim struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	exec eximBinary
}

func (e *Exim) Configuration() any {
	return e.Config
}

func (e *Exim) Init() error {
	exim, err := e.initEximExec()
	if err != nil {
		return fmt.Errorf("exim exec initialization: %v", err)
	}
	e.exec = exim

	return nil
}

func (e *Exim) Check() error {
	mx, err := e.collect()
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (e *Exim) Charts() *module.Charts {
	return e.charts
}

func (e *Exim) Collect() map[string]int64 {
	mx, err := e.collect()
	if err != nil {
		e.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (e *Exim) Cleanup() {}
