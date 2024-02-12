// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/pkg/tlscfg"
	"github.com/netdata/go.d.plugin/pkg/web"

	cfssllog "github.com/cloudflare/cfssl/log"
	"github.com/netdata/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	cfssllog.Level = cfssllog.LevelFatal
	module.Register("x509check", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 60,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *X509Check {
	return &X509Check{
		Config: Config{
			Timeout:           web.Duration{Duration: time.Second * 2},
			DaysUntilWarn:     14,
			DaysUntilCritical: 7,
		},
	}
}

type Config struct {
	Source            string
	Timeout           web.Duration
	tlscfg.TLSConfig  `yaml:",inline"`
	DaysUntilWarn     int64 `yaml:"days_until_expiration_warning"`
	DaysUntilCritical int64 `yaml:"days_until_expiration_critical"`
	CheckRevocation   bool  `yaml:"check_revocation_status"`
}

type X509Check struct {
	module.Base
	Config `yaml:",inline"`
	charts *module.Charts
	prov   provider
}

func (x *X509Check) Init() bool {
	if err := x.validateConfig(); err != nil {
		x.Errorf("config validation: %v", err)
		return false
	}

	prov, err := x.initProvider()
	if err != nil {
		x.Errorf("certificate provider init: %v", err)
		return false
	}
	x.prov = prov

	x.charts = x.initCharts()

	return true
}

func (x *X509Check) Check() bool {
	return len(x.Collect()) > 0
}

func (x *X509Check) Charts() *module.Charts {
	return x.charts
}

func (x *X509Check) Collect() map[string]int64 {
	mx, err := x.collect()
	if err != nil {
		x.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (x *X509Check) Cleanup() {}
