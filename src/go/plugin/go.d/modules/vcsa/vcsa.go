// SPDX-License-Identifier: GPL-3.0-or-later

package vcsa

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
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
		Config: func() any { return &Config{} },
	})
}

func New() *VCSA {
	return &VCSA{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 5),
				},
			},
		},
		charts: vcsaHealthCharts.Copy(),
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type (
	VCSA struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		client healthClient
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

func (vc *VCSA) Configuration() any {
	return vc.Config
}

func (vc *VCSA) Init() error {
	if err := vc.validateConfig(); err != nil {
		vc.Error(err)
		return err
	}

	c, err := vc.initHealthClient()
	if err != nil {
		vc.Errorf("error on creating health client : %vc", err)
		return err
	}
	vc.client = c

	vc.Debugf("using URL %s", vc.URL)
	vc.Debugf("using timeout: %s", vc.Timeout)

	return nil
}

func (vc *VCSA) Check() error {
	err := vc.client.Login()
	if err != nil {
		vc.Error(err)
		return err
	}

	mx, err := vc.collect()
	if err != nil {
		vc.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
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
