// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn

import (
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/modules/openvpn/client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("openvpn", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *OpenVPN {
	return &OpenVPN{
		Config: Config{
			Address: "127.0.0.1:7505",
			Timeout: confopt.Duration(time.Second),
		},

		charts:         charts.Copy(),
		collectedUsers: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery  int                `yaml:"update_every,omitempty" json:"update_every"`
	Address      string             `yaml:"address" json:"address"`
	Timeout      confopt.Duration   `yaml:"timeout,omitempty" json:"timeout"`
	PerUserStats matcher.SimpleExpr `yaml:"per_user_stats,omitempty" json:"per_user_stats"`
}

type (
	OpenVPN struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *Charts

		client openVPNClient

		collectedUsers map[string]bool
		perUserMatcher matcher.Matcher
	}
	openVPNClient interface {
		socket.Client
		Version() (*client.Version, error)
		LoadStats() (*client.LoadStats, error)
		Users() (client.Users, error)
	}
)

func (o *OpenVPN) Configuration() any {
	return o.Config
}

func (o *OpenVPN) Init() error {
	if err := o.validateConfig(); err != nil {
		return err
	}

	m, err := o.initPerUserMatcher()
	if err != nil {
		return err
	}
	o.perUserMatcher = m

	o.client = o.initClient()

	o.Infof("using address: %s, timeout: %s", o.Address, o.Timeout)

	return nil
}

func (o *OpenVPN) Check() error {
	if err := o.client.Connect(); err != nil {
		return err
	}
	defer func() { _ = o.client.Disconnect() }()

	ver, err := o.client.Version()
	if err != nil {
		o.Cleanup()
		return err
	}

	o.Infof("connected to OpenVPN v%d.%d.%d, Management v%d", ver.Major, ver.Minor, ver.Patch, ver.Management)

	return nil
}

func (o *OpenVPN) Charts() *Charts { return o.charts }

func (o *OpenVPN) Collect() map[string]int64 {
	mx, err := o.collect()
	if err != nil {
		o.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (o *OpenVPN) Cleanup() {
	if o.client == nil {
		return
	}
	_ = o.client.Disconnect()
}
