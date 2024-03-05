// SPDX-License-Identifier: GPL-3.0-or-later

package unbound

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/socket"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/tlscfg"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("unbound", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Unbound {
	return &Unbound{
		Config: Config{
			Address:    "127.0.0.1:8953",
			ConfPath:   "/etc/unbound/unbound.conf",
			Timeout:    web.Duration(time.Second),
			Cumulative: false,
			UseTLS:     true,
			TLSConfig: tlscfg.TLSConfig{
				TLSCert:            "/etc/unbound/unbound_control.pem",
				TLSKey:             "/etc/unbound/unbound_control.key",
				InsecureSkipVerify: true,
			},
		},
		curCache: newCollectCache(),
		cache:    newCollectCache(),
	}
}

type Config struct {
	tlscfg.TLSConfig `yaml:",inline" json:""`
	UpdateEvery      int          `yaml:"update_every" json:"update_every"`
	Address          string       `yaml:"address" json:"address"`
	ConfPath         string       `yaml:"conf_path" json:"conf_path"`
	Timeout          web.Duration `yaml:"timeout" json:"timeout"`
	Cumulative       bool         `yaml:"cumulative_stats" json:"cumulative_stats"`
	UseTLS           bool         `yaml:"use_tls" json:"use_tls"`
}

type Unbound struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	client socket.Client

	cache            collectCache
	curCache         collectCache
	prevCacheMiss    float64 // needed for cumulative mode
	extChartsCreated bool
}

func (u *Unbound) Configuration() any {
	return u.Config
}

func (u *Unbound) Init() error {
	if enabled := u.initConfig(); !enabled {
		return errors.New("remote control is disabled in the configuration file")
	}

	if err := u.initClient(); err != nil {
		u.Errorf("creating client: %v", err)
		return err
	}

	u.charts = charts(u.Cumulative)

	u.Debugf("using address: %s, cumulative: %v, use_tls: %v, timeout: %s", u.Address, u.Cumulative, u.UseTLS, u.Timeout)
	if u.UseTLS {
		u.Debugf("using tls_skip_verify: %v, tls_key: %s, tls_cert: %s", u.InsecureSkipVerify, u.TLSKey, u.TLSCert)
	}

	return nil
}

func (u *Unbound) Check() error {
	mx, err := u.collect()
	if err != nil {
		u.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (u *Unbound) Charts() *module.Charts {
	return u.charts
}

func (u *Unbound) Collect() map[string]int64 {
	mx, err := u.collect()
	if err != nil {
		u.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (u *Unbound) Cleanup() {
	if u.client != nil {
		_ = u.client.Disconnect()
	}
}
