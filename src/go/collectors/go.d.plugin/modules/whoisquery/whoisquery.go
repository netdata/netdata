// SPDX-License-Identifier: GPL-3.0-or-later

package whoisquery

import (
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
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
	})
}

func New() *WhoisQuery {
	return &WhoisQuery{
		Config: Config{
			Timeout:       web.Duration{Duration: time.Second * 5},
			DaysUntilWarn: 90,
			DaysUntilCrit: 30,
		},
	}
}

type Config struct {
	Source        string
	Timeout       web.Duration `yaml:"timeout"`
	DaysUntilWarn int64        `yaml:"days_until_expiration_warning"`
	DaysUntilCrit int64        `yaml:"days_until_expiration_critical"`
}

type WhoisQuery struct {
	module.Base
	Config `yaml:",inline"`

	charts *module.Charts

	prov provider
}

func (w *WhoisQuery) Init() bool {
	if err := w.validateConfig(); err != nil {
		w.Errorf("config validation: %v", err)
		return false
	}

	prov, err := w.initProvider()
	if err != nil {
		w.Errorf("init whois provider: %v", err)
		return false
	}
	w.prov = prov

	w.charts = w.initCharts()

	return true
}

func (w *WhoisQuery) Check() bool {
	return len(w.Collect()) > 0
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
