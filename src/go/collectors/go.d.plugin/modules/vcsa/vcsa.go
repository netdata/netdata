// SPDX-License-Identifier: GPL-3.0-or-later

package vcsa

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/netdata/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("vcsa", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5, // VCSA health checks freq is 5 second.
		},
		Create: func() module.Module { return New() },
	})
}

func New() *VCSA {
	return &VCSA{
		Config: Config{
			HTTP: web.HTTP{
				Client: web.Client{
					Timeout: web.Duration{Duration: time.Second * 5},
				},
			},
		},
		charts: vcsaHealthCharts.Copy(),
	}
}

type Config struct {
	web.HTTP `yaml:",inline"`
}

type (
	VCSA struct {
		module.Base
		Config `yaml:",inline"`

		client healthClient

		charts *module.Charts
	}

	healthClient interface {
		Login() error
		Logout() error
		Ping() error
		ApplMgmt() (string, error)
		DatabaseStorage() (string, error)
		Load() (string, error)
		Mem() (string, error)
		SoftwarePackages() (string, error)
		Storage() (string, error)
		Swap() (string, error)
		System() (string, error)
	}
)

func (vc *VCSA) Init() bool {
	if err := vc.validateConfig(); err != nil {
		vc.Error(err)
		return false
	}

	c, err := vc.initHealthClient()
	if err != nil {
		vc.Errorf("error on creating health client : %vc", err)
		return false
	}
	vc.client = c

	vc.Debugf("using URL %s", vc.URL)
	vc.Debugf("using timeout: %s", vc.Timeout.Duration)

	return true
}

func (vc *VCSA) Check() bool {
	err := vc.client.Login()
	if err != nil {
		vc.Error(err)
		return false
	}

	return len(vc.Collect()) > 0
}

func (vc *VCSA) Charts() *module.Charts {
	return vc.charts
}

func (vc *VCSA) Collect() map[string]int64 {
	mx, err := vc.collect()
	if err != nil {
		vc.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (vc *VCSA) Cleanup() {
	if vc.client == nil {
		return
	}
	err := vc.client.Logout()
	if err != nil {
		vc.Errorf("error on logout : %v", err)
	}
}
