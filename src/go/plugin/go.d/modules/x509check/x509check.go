// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	cfssllog "github.com/cloudflare/cfssl/log"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
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
		Config: func() any { return &Config{} },
	})
}

func New() *X509Check {
	return &X509Check{
		Config: Config{
			Timeout:           web.Duration(time.Second * 2),
			DaysUntilWarn:     14,
			DaysUntilCritical: 7,
		},
	}
}

type Config struct {
	UpdateEvery       int          `yaml:"update_every,omitempty" json:"update_every"`
	Source            string       `yaml:"source" json:"source"`
	Timeout           web.Duration `yaml:"timeout,omitempty" json:"timeout"`
	DaysUntilWarn     int64        `yaml:"days_until_expiration_warning,omitempty" json:"days_until_expiration_warning"`
	DaysUntilCritical int64        `yaml:"days_until_expiration_critical,omitempty" json:"days_until_expiration_critical"`
	CheckRevocation   bool         `yaml:"check_revocation_status" json:"check_revocation_status"`
	tlscfg.TLSConfig  `yaml:",inline" json:""`
}

type X509Check struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	prov provider
}

func (x *X509Check) Configuration() any {
	return x.Config
}

func (x *X509Check) Init() error {
	if err := x.validateConfig(); err != nil {
		x.Errorf("config validation: %v", err)
		return err
	}

	prov, err := x.initProvider()
	if err != nil {
		x.Errorf("certificate provider init: %v", err)
		return err
	}
	x.prov = prov

	x.charts = x.initCharts()

	return nil
}

func (x *X509Check) Check() error {
	mx, err := x.collect()
	if err != nil {
		x.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
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
