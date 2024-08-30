// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn_status_log

import (
	_ "embed"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("openvpn_status_log", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *OpenVPNStatusLog {
	return &OpenVPNStatusLog{
		Config: Config{
			LogPath: "/var/log/openvpn/status.log",
		},
		charts:         charts.Copy(),
		collectedUsers: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery  int                `yaml:"update_every,omitempty" json:"update_every"`
	LogPath      string             `yaml:"log_path" json:"log_path"`
	PerUserStats matcher.SimpleExpr `yaml:"per_user_stats,omitempty" json:"per_user_stats"`
}

type OpenVPNStatusLog struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	perUserMatcher matcher.Matcher
	collectedUsers map[string]bool
}

func (o *OpenVPNStatusLog) Configuration() any {
	return o.Config
}

func (o *OpenVPNStatusLog) Init() error {
	if err := o.validateConfig(); err != nil {
		o.Errorf("error on validating config: %v", err)
		return err
	}

	m, err := o.initPerUserStatsMatcher()
	if err != nil {
		o.Errorf("error on creating 'per_user_stats' matcher: %v", err)
		return err
	}
	if m != nil {
		o.perUserMatcher = m
	}

	return nil
}

func (o *OpenVPNStatusLog) Check() error {
	mx, err := o.collect()
	if err != nil {
		o.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (o *OpenVPNStatusLog) Charts() *module.Charts {
	return o.charts
}

func (o *OpenVPNStatusLog) Collect() map[string]int64 {
	mx, err := o.collect()
	if err != nil {
		o.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (o *OpenVPNStatusLog) Cleanup() {}
