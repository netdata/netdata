// SPDX-License-Identifier: GPL-3.0-or-later

package whoisquery

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("whoisquery", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 60,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *WhoisQuery {
	return &WhoisQuery{
		Config: Config{
			Timeout:       web.Duration(time.Second * 5),
			DaysUntilWarn: 30,
			DaysUntilCrit: 15,
		},
	}
}

type Config struct {
	UpdateEvery   int          `yaml:"update_every,omitempty" json:"update_every"`
	Source        string       `yaml:"source" json:"source"`
	Timeout       web.Duration `yaml:"timeout,omitempty" json:"timeout"`
	DaysUntilWarn int64        `yaml:"days_until_expiration_warning,omitempty" json:"days_until_expiration_warning"`
	DaysUntilCrit int64        `yaml:"days_until_expiration_critical,omitempty" json:"days_until_expiration_critical"`
}

type WhoisQuery struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	prov provider
}

func (w *WhoisQuery) Configuration() any {
	return w.Config
}

func (w *WhoisQuery) Init() error {
	if err := w.validateConfig(); err != nil {
		w.Errorf("config validation: %v", err)
		return err
	}

	prov, err := w.initProvider()
	if err != nil {
		w.Errorf("init whois provider: %v", err)
		return err
	}
	w.prov = prov

	w.charts = w.initCharts()

	return nil
}

func (w *WhoisQuery) Check() error {
	mx, err := w.collect()
	if err != nil {
		w.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (w *WhoisQuery) Charts() *module.Charts {
	return w.charts
}

func (w *WhoisQuery) Collect() map[string]int64 {
	mx, err := w.collect()
	if err != nil {
		w.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (w *WhoisQuery) Cleanup() {}
