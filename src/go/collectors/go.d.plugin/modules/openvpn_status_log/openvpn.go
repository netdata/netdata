// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn_status_log

import (
	_ "embed"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/matcher"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("openvpn_status_log", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *OpenVPNStatusLog {
	config := Config{
		LogPath: "/var/log/openvpn/status.log",
	}
	return &OpenVPNStatusLog{
		Config:         config,
		charts:         charts.Copy(),
		collectedUsers: make(map[string]bool),
	}
}

type Config struct {
	LogPath      string             `yaml:"log_path"`
	PerUserStats matcher.SimpleExpr `yaml:"per_user_stats"`
}

type OpenVPNStatusLog struct {
	module.Base

	Config `yaml:",inline"`

	charts *module.Charts

	collectedUsers map[string]bool
	perUserMatcher matcher.Matcher
}

func (o *OpenVPNStatusLog) Init() bool {
	if err := o.validateConfig(); err != nil {
		o.Errorf("error on validating config: %v", err)
		return false
	}

	m, err := o.initPerUserStatsMatcher()
	if err != nil {
		o.Errorf("error on creating 'per_user_stats' matcher: %v", err)
		return false
	}

	if m != nil {
		o.perUserMatcher = m
	}

	return true
}

func (o *OpenVPNStatusLog) Check() bool {
	return len(o.Collect()) > 0
}

func (o OpenVPNStatusLog) Charts() *module.Charts {
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
