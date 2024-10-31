// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package fail2ban

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
	module.Register("fail2ban", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Fail2Ban {
	return &Fail2Ban{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
		},
		charts:        &module.Charts{},
		discoverEvery: time.Minute * 5,
		seenJails:     make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type (
	Fail2Ban struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec fail2banClientCli

		discoverEvery    time.Duration
		lastDiscoverTime time.Time
		forceDiscover    bool
		jails            []string

		seenJails map[string]bool
	}
	fail2banClientCli interface {
		status() ([]byte, error)
		jailStatus(s string) ([]byte, error)
	}
)

func (f *Fail2Ban) Configuration() any {
	return f.Config
}

func (f *Fail2Ban) Init() error {
	f2bClientExec, err := f.initFail2banClientCliExec()
	if err != nil {
		f.Errorf("fail2ban-client exec initialization: %v", err)
		return err
	}
	f.exec = f2bClientExec

	return nil
}

func (f *Fail2Ban) Check() error {
	mx, err := f.collect()
	if err != nil {
		f.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (f *Fail2Ban) Charts() *module.Charts {
	return f.charts
}

func (f *Fail2Ban) Collect() map[string]int64 {
	mx, err := f.collect()
	if err != nil {
		f.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (f *Fail2Ban) Cleanup() {}
