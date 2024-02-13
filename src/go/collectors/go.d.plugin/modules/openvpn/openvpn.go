// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/modules/openvpn/client"
	"github.com/netdata/go.d.plugin/pkg/matcher"
	"github.com/netdata/go.d.plugin/pkg/socket"
	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/netdata/go.d.plugin/agent/module"
)

const (
	defaultAddress        = "127.0.0.1:7505"
	defaultConnectTimeout = time.Second * 2
	defaultReadTimeout    = time.Second * 2
	defaultWriteTimeout   = time.Second * 2
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("openvpn", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			Disabled: true,
		},
		Create: func() module.Module { return New() },
	})
}

// New creates OpenVPN with default values.
func New() *OpenVPN {
	config := Config{
		Address:        defaultAddress,
		ConnectTimeout: web.Duration{Duration: defaultConnectTimeout},
		ReadTimeout:    web.Duration{Duration: defaultReadTimeout},
		WriteTimeout:   web.Duration{Duration: defaultWriteTimeout},
	}
	return &OpenVPN{
		Config:         config,
		charts:         charts.Copy(),
		collectedUsers: make(map[string]bool),
	}
}

// Config is the OpenVPN module configuration.
type Config struct {
	Address        string
	ConnectTimeout web.Duration       `yaml:"connect_timeout"`
	ReadTimeout    web.Duration       `yaml:"read_timeout"`
	WriteTimeout   web.Duration       `yaml:"write_timeout"`
	PerUserStats   matcher.SimpleExpr `yaml:"per_user_stats"`
}

type openVPNClient interface {
	socket.Client
	Version() (*client.Version, error)
	LoadStats() (*client.LoadStats, error)
	Users() (client.Users, error)
}

// OpenVPN OpenVPN module.
type OpenVPN struct {
	module.Base
	Config         `yaml:",inline"`
	client         openVPNClient
	charts         *Charts
	collectedUsers map[string]bool
	perUserMatcher matcher.Matcher
}

// Cleanup makes cleanup.
func (o *OpenVPN) Cleanup() {
	if o.client == nil {
		return
	}
	_ = o.client.Disconnect()
}

// Init makes initialization.
func (o *OpenVPN) Init() bool {
	if !o.PerUserStats.Empty() {
		m, err := o.PerUserStats.Parse()
		if err != nil {
			o.Errorf("error on creating per user stats matcher : %v", err)
			return false
		}
		o.perUserMatcher = matcher.WithCache(m)
	}

	config := socket.Config{
		Address:        o.Address,
		ConnectTimeout: o.ConnectTimeout.Duration,
		ReadTimeout:    o.ReadTimeout.Duration,
		WriteTimeout:   o.WriteTimeout.Duration,
	}
	o.client = &client.Client{Client: socket.New(config)}

	o.Infof("using address: %s, connect timeout: %s, read timeout: %s, write timeout: %s",
		o.Address, o.ConnectTimeout.Duration, o.ReadTimeout.Duration, o.WriteTimeout.Duration)

	return true
}

// Check makes check.
func (o *OpenVPN) Check() bool {
	if err := o.client.Connect(); err != nil {
		o.Error(err)
		return false
	}
	defer func() { _ = o.client.Disconnect() }()

	ver, err := o.client.Version()
	if err != nil {
		o.Error(err)
		o.Cleanup()
		return false
	}

	o.Infof("connected to OpenVPN v%d.%d.%d, Management v%d", ver.Major, ver.Minor, ver.Patch, ver.Management)
	return true
}

// Charts creates Charts.
func (o OpenVPN) Charts() *Charts { return o.charts }

// Collect collects metrics.
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
